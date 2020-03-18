package actor

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/protolambda/rumor/rpc/methods"
	"github.com/protolambda/rumor/rpc/reqresp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RPCState struct {
	Goodbye *Responder
	Status *Responder
	BlocksByRange *Responder
	BlocksByRoot *Responder
}

type RequestKey uint64

type RequestEntry struct {
	From peer.ID
	handler reqresp.RequestResponder
	cancel func()
}

type Responder struct {
	keyCounter RequestKey
	keyCounterMutex sync.Mutex
	// RequestKey -> RequestEntry
	Requests sync.Map
}

func (r *Responder) GetRequest(key RequestKey) *RequestEntry {
	e, ok := r.Requests.Load(key)
	if ok {
		return e.(*RequestEntry)
	} else {
		return nil
	}
}

func (r *Responder) CloseRequest(key RequestKey) {
	e := r.GetRequest(key)
	if e == nil {
		return
	}
	e.cancel()
	r.Requests.Delete(key)
}

func (r *Responder) AddRequest(req *RequestEntry) RequestKey {
	r.keyCounterMutex.Lock()
	key := r.keyCounter
	r.keyCounter += 1
	r.keyCounterMutex.Unlock()
	r.Requests.Store(key, req)
	return key
}

/*
TODO:
- refactor "req" command
- generic "listen"/"req"/"resp" command; take protocol-id and ssz-bytes as argument
- implement "resp" command, take ssz-bytes or type-specific input

 */

func (r *Actor) InitRpcCmd(log logrus.FieldLogger, state *RPCState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rpc",
		Short: "Manage Eth2 RPC",
	}

	readOptionalComp := func(cmd *cobra.Command) (reqresp.Compression, error) {
		if compStr, err := cmd.Flags().GetString("compression"); err != nil {
			return nil, err
		} else {
			switch compStr {
			case "none", "", "false":
				// no compression
				return nil, nil
			case "snappy":
				return reqresp.SnappyCompression{}, nil
			default:
				return nil, fmt.Errorf("cannot recognize compression '%s'", compStr)
			}
		}
	}
	// TODO: stop responses command

	prepareReqFn := func(cmd *cobra.Command, m *reqresp.RPCMethod) func(peerIDStr string, reqInput reqresp.RequestInput) {
		cmd.Flags().String("compression", "none", "Optional compression. Try 'snappy' for streaming-snappy")
		var maxChunks uint64
		cmd.Flags().Uint64Var(&maxChunks, "max-chunks", m.DefaultResponseChunkCount, "Max response chunk count, if 0, do not wait for a response at all.")
		var timeout uint64
		cmd.Flags().Uint64Var(&timeout, "timeout", 10_000, "Apply timeout of n milliseconds the stream (complete request <> response time). 0 to Disable timeout.")

		return func(peerIDStr string, reqInput reqresp.RequestInput) {
			if r.NoHost(log) {
				return
			}
			sFn := reqresp.NewStreamFn(func(ctx context.Context, peerId peer.ID, protocolId protocol.ID) (network.Stream, error) {
				return r.P2PHost.NewStream(ctx, peerId, protocolId)
			}).WithTimeout(time.Second * 10) // TODO

			ctx := r.Ctx // TODO; change to command time-out
			if timeout != 0 {
				ctx, _ = context.WithTimeout(ctx, time.Millisecond*time.Duration(timeout))
			}
			peerID, err := peer.Decode(peerIDStr)
			if err != nil {
				log.Error(err)
				return
			}
			comp, err := readOptionalComp(cmd)
			if err != nil {
				log.Error(err)
				return
			}

			lastRespChunkIndex := int64(-1)
			if err := m.RunRequest(ctx, sFn, peerID, comp, reqInput, maxChunks,
				func(chunkIndex uint64, readChunk func(dest interface{}) error) error {
					log.Debugf("Received response chunk %d from peer %s", chunkIndex, peerID.Pretty())
					lastRespChunkIndex = int64(chunkIndex)
					return onResp(peerID, chunkIndex, readChunk)
				}, func(chunkIndex uint64, msg string) error {
					log.Errorf("request (protocol %s) to %s was turned down at chunk %d because of INVALID INPUT. Error message from server: %s", m.Protocol, peerID, chunkIndex, msg)
					return nil
				}, func(chunkIndex uint64, msg string) error {
					log.Errorf("request (protocol %s) to %s was turned down at chunk %d because of SERVER ERROR. Error message from server: %s", m.Protocol, peerID, chunkIndex, msg)
					return nil
				}, func() {
					log.Debugf("Responses of peer %s stopped after %d response chunks", peerID.Pretty(), lastRespChunkIndex+1)
					onClose(peerID)
				}); err != nil {
				log.Errorf("failed request: %v", err)
			}
		}
	}

	makeListenCmd := func(
		responder *Responder,
		cmd *cobra.Command,
		m *reqresp.RPCMethod,
	) {
		cmd.Flags().String("compression", "none", "Optional compression. Try 'snappy' for streaming-snappy")
		var drop bool
		cmd.Flags().BoolVar(&drop, "drop", m.DefaultResponseChunkCount == 0, "Drop the requests, do not queue for a response.")
		var raw bool
		cmd.Flags().BoolVar(&raw, "raw", false, "Do not decode the request, look at raw bytes")
		var timeout uint64
		cmd.Flags().Uint64Var(&timeout, "timeout", 10_000, "Apply timeout of n milliseconds to each stream (complete request <> response time). 0 to Disable timeout.")
		cmd.Run = func(cmd *cobra.Command, args []string) {
			if r.NoHost(log) {
				return
			}
			// TODO; switch to command timeout.
			sCtxFn := func() context.Context {
				if timeout == 0 {
					return r.Ctx
				}
				ctx, _ := context.WithTimeout(r.Ctx, time.Millisecond * time.Duration(timeout))
				return ctx
			}
			comp, err := readOptionalComp(cmd)
			if err != nil {
				log.Error(err)
				return
			}
			listenReq := func(ctx context.Context, peerId peer.ID, handler reqresp.ChunkedRequestHandler) {
				var data interface{}
				var inputErr error
				if raw {
					bytez, err := handler.RawRequest()
					if err != nil {
						inputErr = err
					} else {
						data = hex.EncodeToString(bytez)
					}
				} else {
					reqObj := m.RequestCodec.Alloc()
					err := handler.ReadRequest(reqObj)
					if err != nil {
						inputErr = err
					} else {
						data = reqObj
					}
				}

				if drop {
					log.WithFields(logrus.Fields{
						"from":      peerId.String(),
						"protocol":  m.Protocol,
						"input_err": inputErr,
						"data":      data,
					}).Infof("Received request, dropping it!")
				} else {
					ctx, cancel := context.WithCancel(ctx)
					reqId := responder.AddRequest(&RequestEntry{
						From:                    peerId,
						handler:                 handler,
						cancel:                  cancel,
					})

					log.WithFields(logrus.Fields{
						"req_id":    reqId,
						"from":      peerId.String(),
						"protocol":  m.Protocol,
						"input_err": inputErr,
						"data":      data,
					}).Infof("Received request, queued it to respond to!")

					// Wait for context to stop processing the request (stream will be closed after return)
					<-ctx.Done()
				}
			}
			streamHandler, err := m.MakeStreamHandler(sCtxFn, comp, listenReq)
			if err != nil {
				log.Error(err)
				return
			}
			r.P2PHost.SetStreamHandler(m.Protocol, streamHandler)
		}
	}

	checkAndGetReq := func(reqKeyStr string, responder *Responder) (key RequestKey, req *RequestEntry, ok bool) {
		if r.NoHost(log) {
			return 0, nil, false
		}
		reqId, err := strconv.ParseUint(reqKeyStr, 0, 64)
		if err != nil {
			log.Errorf("Could not parse request key '%s'", reqKeyStr)
			return 0, nil, false
		}

		key = RequestKey(reqId)
		req = responder.GetRequest(key)
		if req == nil {
			log.Errorf("Could not find request corresponding to key '%s'", key)
			return 0, nil, false
		}
		return key, req, true
	}

	makeRawRespChunkCmd := func(
		responder *Responder,
		cmd *cobra.Command,
		doneDefault bool,
	) {
		var done bool
		cmd.Flags().BoolVar(&done, "done", doneDefault, "After writing this chunk, close the response (no more chunks).")
		cmd.Args = cobra.ExactArgs(2)
		cmd.Run = func(cmd *cobra.Command, args []string) {
			key, req, ok := checkAndGetReq(args[0], responder)
			if !ok {
				return
			}
			byteStr := args[1]
			if strings.HasPrefix(byteStr, "0x") {
				byteStr = byteStr[2:]
			}
			bytez, err := hex.DecodeString(byteStr)
			if err == nil {
				log.Errorf("Data is not a valid hex-string: '%s'", byteStr)
				return
			}

			if err := req.handler.WriteRawResponseChunk(bytez); err != nil {
				log.Error(err)
				return
			}

			if done {
				responder.CloseRequest(key)
			}
		}
	}

	makeInvalidInputCmd := func(
		responder *Responder,
	) *cobra.Command {
		cmd := &cobra.Command{
			Use:   "invalid-input <request-ID> <message>",
			Short: "Respond with an invalid-input message chunk",
		}
		var done bool
		cmd.Flags().BoolVar(&done, "done", true, "After writing this chunk, close the response (no more chunks).")
		cmd.Args = cobra.ExactArgs(2)
		cmd.Run = func(cmd *cobra.Command, args []string) {
			key, req, ok := checkAndGetReq(args[0], responder)
			if !ok {
				return
			}
			if err := req.handler.WriteInvalidMsgChunk(args[1]); err != nil {
				log.Error(err)
				return
			}
			if done {
				responder.CloseRequest(key)
			}
		}
		return cmd
	}

	makeServerErrorCmd := func(
		responder *Responder,
	) *cobra.Command {
		cmd := &cobra.Command{
			Use:   "server-error <request-ID> <message>",
			Short: "Respond with a server-error message chunk",
		}
		var done bool
		cmd.Flags().BoolVar(&done, "done", true, "After writing this chunk, close the response (no more chunks).")
		cmd.Args = cobra.ExactArgs(2)
		cmd.Run = func(cmd *cobra.Command, args []string) {
			key, req, ok := checkAndGetReq(args[0], responder)
			if !ok {
				return
			}
			if err := req.handler.WriteServerErrorChunk(args[1]); err != nil {
				log.Error(err)
				return
			}
			if done {
				responder.CloseRequest(key)
			}
		}
		return cmd
	}

	/*
	<rpc type name>   # for 'goodbye', 'status', 'blocks-by-range', 'blocks-by-root'
	      req
				with --compression=none --max-chunks=20 <peerID> [args to parse into request]
				raw  --compression=none --max-chunks=20 <peerID> <hex encoded bytes>
	      listen --compression=none --always-close=false --raw=false                 # queues requests, logs request-id. Closes listener when command is canceled.
	      resp
				chunk
					with --done=false <req-ID> [args to parse into response chunk]
					raw  --done=false --result-code=0 <req-ID> <hex encoded bytes>
				invalid-input --done=true <req-ID> [msg string]
				server-error --done=true <req-ID> [msg string]
		  close <req-ID>                                                   # Cancel an open request, closes the response stream, removes it from memory

	    --result-code can be used to write custom chunk data (Test behavior of unspecced chunk types)
		--drop stops listening for responses immediately after sending the request.
	    --done closes the response immediately after sending the chunk
	    --always-close=true prevents requests from being queued, and requests are immediately dropped

		For 'blocks-by-{range, root} req with';
			--max-chunks, if left unchanged, should change to the request span
		For 'goodbye req {with, raw}';
			--max-chunks=0 as default, do not wait for response
		For 'status req {with, raw}';
			--max-chunks=1 as default, only single response
		For 'status resp chunk {with, raw}';
			--done=true as default, do not write more than one chunk

		Incoming Request logs:
			{
				"req_id":    The ID to respond to it, if queued
				"from":      Peer-ID of request sender
				"protocol":  Protocol string of request
				"input_err": if the request was invalid, this is the error message. The responder can decide to respond with `<name> resp invalid-input <red-ID> "some message here to be nice to the client"`
				"data":      if the request was valid, this is either a hex-encoded string (if listened with --raw) or the parsed request.
			}

		Incoming Response logs:
			{
				W.I.P.
			}
	 */

	makeMethodCmd := func(name string, responder *Responder, m *reqresp.RPCMethod) *cobra.Command {
		methodCmd := &cobra.Command{
			Use:   name,
			Short: fmt.Sprintf("Manage %s RPC", name),
		}
		// Requests
		// -----------------------------------
		reqCmd := &cobra.Command{
			Use:   "req",
			Short: "Make requests",
		}
		reqWithCmd := &cobra.Command{
			Use:   "with <peer-ID>",
			Short: "Build and make a request with the given arguments",
		}
		{
			reqFn := prepareReqFn(reqWithCmd, m)
			reqWithCmd.Run = func(cmd *cobra.Command, args []string) {

				reqInput := reqresp.RequestSSZInput{Obj: req}
				reqFn() // TODO
			}
		}
		reqRawCmd := &cobra.Command{
			Use:   "raw",
			Short: "Make raw requests",
		}
		{
			reqFn := prepareReqFn(reqWithCmd, m)
			reqWithCmd.Run = func(cmd *cobra.Command, args []string) {
				reqInput := reqresp.RequestBytesInput(b)
				reqFn() // TODO
			}
		}
		reqCmd.AddCommand(reqWithCmd, reqRawCmd)
		methodCmd.AddCommand(reqCmd)

		// Listen
		// -----------------------------------
		listenCmd := &cobra.Command{
			Use:   "listen",
			Short: "Listen for new requests",
		}
		makeListenCmd(responder, listenCmd, m)

		methodCmd.AddCommand(listenCmd)

		// Responses
		// -----------------------------------
		respCmd := &cobra.Command{
			Use:   "resp",
			Short: "Respond to requests",
		}
		respChunkCmd := &cobra.Command{
			Use:   "chunk",
			Short: "Respond a chunk to a request",
		}
		respChunkWithCmd := &cobra.Command{
			Use:   "with <request-ID>",
			Short: "Build and make a request with the given arguments",
		}
		// TODO: respChunkWithCmd

		respChunkRawCmd := &cobra.Command{
			Use:   "raw",
			Short: "Make raw requests",
		}
		makeRawRespChunkCmd(responder, respChunkRawCmd, m.DefaultResponseChunkCount > 1)
		respChunkCmd.AddCommand(respChunkWithCmd, respChunkRawCmd)

		respInvalidInputCmd := makeInvalidInputCmd(responder)
		respServerErrorCmd := makeServerErrorCmd(responder)

		respCmd.AddCommand(respChunkCmd, respInvalidInputCmd, respServerErrorCmd)
		methodCmd.AddCommand(respCmd)

		// Close
		// -----------------------------------
		closeCmd := &cobra.Command{
			Use:   "close",
			Short: "Close open requests",
		}
		// TODO
		methodCmd.AddCommand(closeCmd)

		return methodCmd
	}

	/* <goodbye-code>
		v, err := strconv.ParseUint(args[1], 0, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Goodbye code '%s'", args[1])
		}
		req := methods.Goodbye(v)
		return &req, nil


	<start-slot> <count> <step> [head-root-hex]
		startSlot, err := strconv.ParseUint(args[1], 0, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse start slot '%s'", args[1])
		}
		count, err := strconv.ParseUint(args[2], 0, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse count '%s'", args[1])
		}
		step, err := strconv.ParseUint(args[3], 0, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse step '%s'", args[1])
		}
		v2, err := cmd.Flags().GetBool("v2")
		if err != nil {
			return nil, err
		}
		if v2 {
			return &methods.BlocksByRangeReqV2{
				StartSlot: methods.Slot(startSlot),
				Count:     count,
				Step:      step,
			}, nil
		} else {
			var root [32]byte
			if len(args) > 4 {
				root, err = parseRoot(args[4])
				if err != nil {
					return nil, err
				}
			}
			return &methods.BlocksByRangeReqV1{
				HeadBlockRoot: root,
				StartSlot:     methods.Slot(startSlot),
				Count:         count,
				Step:          step,
			}, nil
		}
	 */



	parseRoot := func(v string) ([32]byte, error) {
		if v == "0" {
			return [32]byte{}, nil
		}
		if strings.HasPrefix(v, "0x") {
			v = v[2:]
		}
		if len(v) != 64 {
			return [32]byte{}, fmt.Errorf("provided root has length %d, expected 64 hex characters (ignoring optional 0x prefix)", len(v))
		}
		var out [32]byte
		_, err := hex.Decode(out[:], []byte(v))
		return out, err
	}
	parseForkVersion := func(v string) ([4]byte, error) {
		if strings.HasPrefix(v, "0x") {
			v = v[2:]
		}
		if len(v) != 8 {
			return [4]byte{}, fmt.Errorf("provided root has length %d, expected 8 hex characters (ignoring optional 0x prefix)", len(v))
		}
		var out [4]byte
		_, err := hex.Decode(out[:], []byte(v))
		return out, err
	}

	// <head-fork-version> <finalized-root> <finalized-epoch> <head-root> <head-slot>
	parseStatus := func(args []string) (*methods.Status, error) {
		forkVersion, err := parseForkVersion(args[0])
		if err != nil {
			return nil, err
		}
		finalizedRoot, err := parseRoot(args[1])
		if err != nil {
			return nil, err
		}
		finalizedEpoch, err := strconv.ParseUint(args[2], 0, 64)
		if err != nil {
			return nil, err
		}
		headRoot, err := parseRoot(args[3])
		if err != nil {
			return nil, err
		}
		headSlot, err := strconv.ParseUint(args[4], 0, 64)
		if err != nil {
			return nil, err
		}
		return &methods.Status{
			HeadForkVersion: forkVersion,
			FinalizedRoot:   finalizedRoot,
			FinalizedEpoch:  methods.Epoch(finalizedEpoch),
			HeadRoot:        headRoot,
			HeadSlot:        methods.Slot(headSlot),
		}, nil
	}

	cmd.AddCommand(makeMethodCmd("goodbye", state.Goodbye, &methods.GoodbyeRPCv1))
	cmd.AddCommand(makeMethodCmd("status", state.Status, &methods.StatusRPCv1))
	cmd.AddCommand(makeMethodCmd("blocks-by-range", state.BlocksByRange, &methods.BlocksByRangeRPCv1))
	cmd.AddCommand(makeMethodCmd("blocks-by-range-v2", state.BlocksByRange, &methods.BlocksByRangeRPCv2))
	cmd.AddCommand(makeMethodCmd("blocks-by-root", state.BlocksByRoot, &methods.BlocksByRootRPCv1))
	return cmd
}

