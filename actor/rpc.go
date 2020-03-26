package actor

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
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
	Goodbye       Responder
	Status        Responder
	BlocksByRange Responder
	BlocksByRoot  Responder
}

type RequestKey uint64

type RequestEntry struct {
	From    peer.ID
	handler reqresp.RequestResponder
	cancel  func()
}

type Responder struct {
	keyCounter      RequestKey
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

func (r *Actor) InitRpcCmd(ctx context.Context, log logrus.FieldLogger, state *RPCState) *cobra.Command {
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

	prepareReqFn := func(cmd *cobra.Command, m *reqresp.RPCMethod) func(peerID peer.ID, reqInput reqresp.RequestInput) {
		cmd.Flags().String("compression", "none", "Optional compression. Try 'snappy' for streaming-snappy")
		var maxChunks uint64
		cmd.Flags().Uint64Var(&maxChunks, "max-chunks", m.DefaultResponseChunkCount, "Max response chunk count, if 0, do not wait for a response at all.")
		var timeout uint64
		cmd.Flags().Uint64Var(&timeout, "timeout", 10_000, "Apply timeout of n milliseconds the stream (complete request <> response time). 0 to Disable timeout.")
		var rawChunks bool
		cmd.Flags().BoolVar(&rawChunks, "raw", false, "If chunks should be logged as raw hex-encoded byte strings")

		return func(peerID peer.ID, reqInput reqresp.RequestInput) {
			if r.NoHost(log) {
				return
			}
			sFn := reqresp.NewStreamFn(r.P2PHost.NewStream)

			reqCtx := ctx
			if timeout != 0 {
				reqCtx, _ = context.WithTimeout(reqCtx, time.Millisecond*time.Duration(timeout))
			}
			comp, err := readOptionalComp(cmd)
			if err != nil {
				log.Error(err)
				return
			}

			if err := m.RunRequest(reqCtx, sFn, peerID, comp, reqInput, maxChunks,
				func(chunk reqresp.ChunkedResponseHandler) error {
					resultCode := chunk.ResultCode()
					f := map[string]interface{}{
						"protocol":    m.Protocol,
						"from":        peerID.String(),
						"chunk_index": chunk.ChunkIndex(),
						"chunk_size":  chunk.ChunkSize(),
						"result_code": resultCode,
					}
					if rawChunks {
						bytez, err := chunk.ReadRaw()
						if err != nil {
							return err
						}
						f["data"] = hex.EncodeToString(bytez)
					} else {
						switch resultCode {
						case reqresp.ServerErrCode, reqresp.InvalidReqCode:
							msg, err := chunk.ReadErrMsg()
							if err != nil {
								return err
							}
							f["msg"] = msg
						case reqresp.SuccessCode:
							data := m.ResponseChunkCodec.Alloc()
							if err := chunk.ReadObj(data); err != nil {
								return err
							}
							f["data"] = data
						default:
							bytez, err := chunk.ReadRaw()
							if err != nil {
								return err
							}
							f["data"] = hex.EncodeToString(bytez)
						}
					}
					log.WithField("chunk", f).Info("Received chunk")
					return nil
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
		var readContents bool
		cmd.Flags().BoolVar(&readContents, "read", true, "Read the contents of the request.")
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
			sCtxFn := func() context.Context {
				if timeout == 0 {
					return ctx
				}
				reqCtx, _ := context.WithTimeout(ctx, time.Millisecond*time.Duration(timeout))
				return reqCtx
			}
			comp, err := readOptionalComp(cmd)
			if err != nil {
				log.Error(err)
				return
			}
			listenReq := func(ctx context.Context, peerId peer.ID, handler reqresp.ChunkedRequestHandler) {
				req := map[string]interface{}{
					"from":     peerId.String(),
					"protocol": m.Protocol,
				}
				if readContents {
					if raw {
						bytez, err := handler.RawRequest()
						if err != nil {
							req["input_err"] = err.Error()
						} else {
							req["data"] = hex.EncodeToString(bytez)
						}
					} else {
						reqObj := m.RequestCodec.Alloc()
						err := handler.ReadRequest(reqObj)
						if err != nil {
							req["input_err"] = err.Error()
						} else {
							req["data"] = reqObj
						}
					}
				}

				if drop {
					log.WithField("req", req).Infof("Received request, dropping it!")
				} else {
					ctx, cancel := context.WithCancel(ctx)
					reqId := responder.AddRequest(&RequestEntry{
						From:    peerId,
						handler: handler,
						cancel:  cancel,
					})
					req["req_id"] = reqId

					log.WithField("req", req).Infof("Received request, queued it to respond to!")

					// Wait for context to stop processing the request (stream will be closed after return)
					<-ctx.Done()
				}
			}
			streamHandler, err := m.MakeStreamHandler(sCtxFn, comp, listenReq)
			if err != nil {
				log.Error(err)
				return
			}
			r.P2PHost.SetStreamHandler(m.Protocol, streamHandler) // TODO add compression to protocol info
			log.WithField("protocol", m.Protocol).Infof("Opened listener")
			<-ctx.Done()
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

	decodeByteStr := func(byteStr string) ([]byte, error) {
		if strings.HasPrefix(byteStr, "0x") {
			byteStr = byteStr[2:]
		}
		return hex.DecodeString(byteStr)
	}

	makeRawRespChunkCmd := func(
		responder *Responder,
		cmd *cobra.Command,
		doneDefault bool,
	) {
		var done bool
		cmd.Flags().BoolVar(&done, "done", doneDefault, "After writing this chunk, close the response (no more chunks).")
		var resultCode uint8
		cmd.Flags().Uint8Var(&resultCode, "result-code", 0, "Customize the chunk result code. (0 = success, 1 = invalid input, 2 = error, 3+ = undefined)")
		cmd.Args = cobra.ExactArgs(2)
		cmd.Run = func(cmd *cobra.Command, args []string) {
			key, req, ok := checkAndGetReq(args[0], responder)
			if !ok {
				return
			}
			byteStr := args[1]
			bytez, err := decodeByteStr(byteStr)
			if err != nil {
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

	makeInvalidRequestCmd := func(
		responder *Responder,
	) *cobra.Command {
		cmd := &cobra.Command{
			Use:   "invalid-request <request-ID> <message>",
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
			if err := req.handler.WriteInvalidRequestChunk(args[1]); err != nil {
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
		//{ TODO
		//	reqFn := prepareReqFn(reqWithCmd, m)
		//	reqWithCmd.Run = func(cmd *cobra.Command, args []string) {
		//		peerID, err := peer.Decode(args[0])
		//		if err != nil {
		//			log.Error(err)
		//			return
		//		}
		//		// TODO parse args/flags into request input
		//		reqInput := reqresp.RequestSSZInput{Obj: req}
		//		reqFn(peerID, reqInput)
		//	}
		//}
		reqRawCmd := &cobra.Command{
			Use:   "raw <peer-ID> <hex-data>",
			Short: "Make raw requests.",
			Args:  cobra.ExactArgs(2),
		}
		{
			reqFn := prepareReqFn(reqRawCmd, m)
			reqRawCmd.Run = func(cmd *cobra.Command, args []string) {
				peerID, err := peer.Decode(args[0])
				if err != nil {
					log.Error(err)
					return
				}
				byteStr := args[1]
				bytez, err := decodeByteStr(byteStr)
				if err != nil {
					log.Errorf("Data is not a valid hex-string: '%s'", byteStr)
					return
				}
				reqInput := reqresp.RequestBytesInput(bytez)
				reqFn(peerID, reqInput)
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
		//respChunkWithCmd := &cobra.Command{
		//	Use:   "with <request-ID>",
		//	Short: "Build and make a request with the given arguments",
		//}
		// TODO: respChunkWithCmd

		respChunkRawCmd := &cobra.Command{
			Use:   "raw",
			Short: "Make raw responses",
		}
		makeRawRespChunkCmd(responder, respChunkRawCmd, m.DefaultResponseChunkCount > 1)
		respChunkCmd.AddCommand(respChunkRawCmd)

		respInvalidInputCmd := makeInvalidRequestCmd(responder)
		respServerErrorCmd := makeServerErrorCmd(responder)

		respCmd.AddCommand(respChunkCmd, respInvalidInputCmd, respServerErrorCmd)
		methodCmd.AddCommand(respCmd)

		// Close
		// -----------------------------------
		closeCmd := &cobra.Command{
			Use:   "close <request-id>",
			Short: "Close open requests",
			Args:  cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				key, req, ok := checkAndGetReq(args[0], responder)
				if !ok {
					return
				}
				responder.CloseRequest(key)
				log.WithFields(logrus.Fields{
					"req_id":   key,
					"peer":     req.From.String(),
					"protocol": m.Protocol,
				}).Infof("Closed request.")
			},
		}
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

	//
	//
	//parseRoot := func(v string) ([32]byte, error) {
	//	if v == "0" {
	//		return [32]byte{}, nil
	//	}
	//	if strings.HasPrefix(v, "0x") {
	//		v = v[2:]
	//	}
	//	if len(v) != 64 {
	//		return [32]byte{}, fmt.Errorf("provided root has length %d, expected 64 hex characters (ignoring optional 0x prefix)", len(v))
	//	}
	//	var out [32]byte
	//	_, err := hex.Decode(out[:], []byte(v))
	//	return out, err
	//}
	//parseForkVersion := func(v string) ([4]byte, error) {
	//	if strings.HasPrefix(v, "0x") {
	//		v = v[2:]
	//	}
	//	if len(v) != 8 {
	//		return [4]byte{}, fmt.Errorf("provided root has length %d, expected 8 hex characters (ignoring optional 0x prefix)", len(v))
	//	}
	//	var out [4]byte
	//	_, err := hex.Decode(out[:], []byte(v))
	//	return out, err
	//}
	//
	//// <head-fork-version> <finalized-root> <finalized-epoch> <head-root> <head-slot>
	//parseStatus := func(args []string) (*methods.Status, error) {
	//	forkVersion, err := parseForkVersion(args[0])
	//	if err != nil {
	//		return nil, err
	//	}
	//	finalizedRoot, err := parseRoot(args[1])
	//	if err != nil {
	//		return nil, err
	//	}
	//	finalizedEpoch, err := strconv.ParseUint(args[2], 0, 64)
	//	if err != nil {
	//		return nil, err
	//	}
	//	headRoot, err := parseRoot(args[3])
	//	if err != nil {
	//		return nil, err
	//	}
	//	headSlot, err := strconv.ParseUint(args[4], 0, 64)
	//	if err != nil {
	//		return nil, err
	//	}
	//	return &methods.Status{
	//		HeadForkVersion: forkVersion,
	//		FinalizedRoot:   finalizedRoot,
	//		FinalizedEpoch:  methods.Epoch(finalizedEpoch),
	//		HeadRoot:        headRoot,
	//		HeadSlot:        methods.Slot(headSlot),
	//	}, nil
	//}

	cmd.AddCommand(makeMethodCmd("goodbye", &state.Goodbye, &methods.GoodbyeRPCv1))
	cmd.AddCommand(makeMethodCmd("status", &state.Status, &methods.StatusRPCv1))
	cmd.AddCommand(makeMethodCmd("blocks-by-range", &state.BlocksByRange, &methods.BlocksByRangeRPCv1))
	cmd.AddCommand(makeMethodCmd("blocks-by-range-v2", &state.BlocksByRange, &methods.BlocksByRangeRPCv2))
	cmd.AddCommand(makeMethodCmd("blocks-by-root", &state.BlocksByRoot, &methods.BlocksByRootRPCv1))
	return cmd
}
