package actor

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/sirupsen/logrus"
	"strconv"
	"sync"
	"time"
)

type RPCState struct {
	Goodbye       Responder
	Status        Responder
	Ping          Responder
	Metadata      Responder
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

type RpcCmd struct {
	*Actor `ask:"-"`
	log    logrus.FieldLogger
}

func (c *RpcCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "goodbye":
		cmd = c.Method("goodbye", &c.RPCState.Goodbye, &methods.GoodbyeRPCv1)
	case "status":
		cmd = c.Method("status", &c.RPCState.Status, &methods.StatusRPCv1)
	case "ping":
		cmd = c.Method("ping", &c.RPCState.Ping, &methods.PingRPCv1)
	case "metadata":
		cmd = c.Method("metadata", &c.RPCState.Metadata, &methods.MetaDataRPCv1)
	case "blocks-by-range":
		cmd = c.Method("blocks-by-range", &c.RPCState.BlocksByRange, &methods.BlocksByRangeRPCv1)
	case "blocks-by-root":
		cmd = c.Method("blocks-by-root", &c.RPCState.BlocksByRoot, &methods.BlocksByRootRPCv1)
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *RpcCmd) Help() string {
	return "Manage Eth2 RPC"
}

func (c *RpcCmd) Method(name string, resp *Responder, method *reqresp.RPCMethod) *RpcMethodCmd {
	return &RpcMethodCmd{
		Actor:     c.Actor,
		log:       c.log,
		Name:      name,
		Responder: resp,
		Method:    method,
	}
}

type RpcMethodCmd struct {
	*Actor    `ask:"-"`
	log       logrus.FieldLogger
	Name      string
	Responder *Responder
	Method    *reqresp.RPCMethod
}

func (c *RpcMethodCmd) Help() string {
	return fmt.Sprintf("Manage %s RPC", c.Name)
}

func (c *RpcMethodCmd) Get(ctx context.Context, args ...string) (cmd interface{}, remaining []string, err error) {
	if len(args) == 0 {
		return nil, nil, errors.New("no subcommand specified")
	}
	switch args[0] {
	case "req":
		cmd = &RpcMethodReqCmd{
			RpcMethodCmd: c,
		}
	case "listen":
		cmd = &RpcMethodListenCmd{
			RpcMethodCmd: c,
			Timeout:      10 * time.Second,
			Compression:  CompressionFlag{Compression: reqresp.SnappyCompression{}},
			Raw:          false,
			Drop:         c.Method.DefaultResponseChunkCount == 0,
			Read:         true,
		}
	case "resp":
		cmd = &RpcMethodRespCmd{
			RpcMethodCmd: c,
		}
	case "close":
		cmd = &RpcMethodCloseCmd{
			RpcMethodCmd: c,
		}
	default:
		return nil, args, fmt.Errorf("unrecognized command: %v", args)
	}
	return cmd, args[1:], nil
}

func (c *RpcMethodCmd) checkAndGetReq(reqKeyStr string) (key RequestKey, req *RequestEntry, err error) {
	reqId, err := strconv.ParseUint(reqKeyStr, 0, 64)
	if err != nil {
		return 0, nil, fmt.Errorf("Could not parse request key '%s'", reqKeyStr)
	}

	key = RequestKey(reqId)
	req = c.Responder.GetRequest(key)
	if req == nil {
		return 0, nil, fmt.Errorf("Could not find request corresponding to key '%s'", key)
	}
	return key, req, nil
}

type RpcMethodReqCmd struct {
	*RpcMethodCmd `ask:"-"`
}

func (c *RpcMethodReqCmd) Help() string {
	return "Make requests"
}

func (c *RpcMethodReqCmd) Get(ctx context.Context, args ...string) (cmd interface{}, remaining []string, err error) {
	if len(args) == 0 {
		return nil, nil, errors.New("no subcommand specified")
	}
	switch args[0] {
	case "raw":
		cmd = &RpcMethodReqRawCmd{
			RpcMethodReqCmd: c,
			Timeout:         10 * time.Second,
			Compression:     CompressionFlag{Compression: reqresp.SnappyCompression{}},
			MaxChunks:       c.Method.DefaultResponseChunkCount,
			Raw:             false,
		}
	default:
		return nil, args, fmt.Errorf("unrecognized command: %v", args)
	}
	return cmd, args[1:], nil
}

type RpcMethodReqRawCmd struct {
	*RpcMethodReqCmd `ask:"-"`
	Timeout          time.Duration   `ask:"--timeout" help:"Timeout for full request and response. 0 to disable"`
	Compression      CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
	MaxChunks        uint64          `ask:"--max-chunks" help:"Max response chunk count, if 0, do not wait for a response at all."`
	Raw              bool            `ask:"--raw" help:"If chunks should be logged as raw hex-encoded byte strings"`
	PeerID           PeerIDFlag      `ask:"<peer-id>" help:"libp2p Peer-ID to request"`
	Data             BytesHexFlag    `ask:"<data>" help:"Raw uncompressed hex-encoded request data"`
}

func (c *RpcMethodReqRawCmd) Help() string {
	return "Make raw requests"
}

func (c *RpcMethodReqRawCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	sFn := reqresp.NewStreamFn(h.NewStream)

	reqCtx := ctx
	if c.Timeout != 0 {
		reqCtx, _ = context.WithTimeout(reqCtx, c.Timeout)
	}

	protocolId := c.Method.Protocol
	if c.Compression.Compression != nil {
		protocolId += protocol.ID("_" + c.Compression.Compression.Name())
	}
	return c.Method.RunRequest(reqCtx, sFn, c.PeerID.PeerID, c.Compression.Compression,
		reqresp.RequestBytesInput(c.Data), c.MaxChunks,
		func(chunk reqresp.ChunkedResponseHandler) error {
			resultCode := chunk.ResultCode()
			f := map[string]interface{}{
				"protocol":    protocolId,
				"from":        c.PeerID.PeerID.String(),
				"chunk_index": chunk.ChunkIndex(),
				"chunk_size":  chunk.ChunkSize(),
				"result_code": resultCode,
			}
			if c.Raw {
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
					data := c.Method.ResponseChunkCodec.Alloc()
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
			c.log.WithField("chunk", f).Info("Received chunk")
			return nil
		})
}

type RpcMethodListenCmd struct {
	*RpcMethodCmd `ask:"-"`
	Timeout       time.Duration   `ask:"--timeout" help:"Apply timeout of n milliseconds to each stream (complete request <> response time). 0 to Disable timeout"`
	Compression   CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
	Raw           bool            `ask:"--raw" help:"Do not decode the request, look at raw bytes"`
	Drop          bool            `ask:"--drop" help:"Drop the requests, do not queue for a response."`
	Read          bool            `ask:"--read" help:"Read the contents of the request."`
}

func (c *RpcMethodListenCmd) Help() string {
	return "Listen for new requests"
}

func (c *RpcMethodListenCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	sCtxFn := func() context.Context {
		if c.Timeout == 0 {
			return ctx
		}
		reqCtx, _ := context.WithTimeout(ctx, c.Timeout)
		return reqCtx
	}
	protocolId := c.Method.Protocol
	if c.Compression.Compression != nil {
		protocolId += protocol.ID("_" + c.Compression.Compression.Name())
	}
	listenReq := func(ctx context.Context, peerId peer.ID, handler reqresp.ChunkedRequestHandler) {
		req := map[string]interface{}{
			"from":     peerId.String(),
			"protocol": protocolId,
		}
		if c.Read {
			if c.Raw {
				bytez, err := handler.RawRequest()
				if err != nil {
					req["input_err"] = err.Error()
				} else {
					req["data"] = hex.EncodeToString(bytez)
				}
			} else {
				reqObj := c.Method.RequestCodec.Alloc()
				err := handler.ReadRequest(reqObj)
				if err != nil {
					req["input_err"] = err.Error()
				} else {
					req["data"] = reqObj
				}
			}
		}

		if c.Drop {
			c.log.WithField("req", req).Infof("Received request, dropping it!")
		} else {
			ctx, cancel := context.WithCancel(ctx)
			reqId := c.Responder.AddRequest(&RequestEntry{
				From:    peerId,
				handler: handler,
				cancel:  cancel,
			})
			req["req_id"] = reqId

			c.log.WithField("req", req).Infof("Received request, queued it to respond to!")

			// Wait for context to stop processing the request (stream will be closed after return)
			<-ctx.Done()
		}
	}
	streamHandler := c.Method.MakeStreamHandler(sCtxFn, c.Compression.Compression, listenReq)
	prot := c.Method.Protocol
	if c.Compression.Compression != nil {
		prot += protocol.ID("_" + c.Compression.Compression.Name())
	}
	h.SetStreamHandler(prot, streamHandler)
	c.log.WithField("started", true).Infof("Opened listener")
	<-ctx.Done()
	// TODO unset stream handler?
	return nil
}

type RpcMethodRespCmd struct {
	*RpcMethodCmd `ask:"-"`
}

func (c *RpcMethodRespCmd) Help() string {
	return "Respond to requests"
}

func (c *RpcMethodRespCmd) Get(ctx context.Context, args ...string) (cmd interface{}, remaining []string, err error) {
	if len(args) == 0 {
		return nil, nil, errors.New("no subcommand specified")
	}
	switch args[0] {
	case "chunk":
		cmd = &RpcMethodRespChunkCmd{
			RpcMethodRespCmd: c,
		}
	case "invalid-request":
		cmd = &RpcMethodRespErrorCmd{
			RpcMethodRespCmd: c,
			Done:             true,
			ResultCode:       reqresp.InvalidReqCode,
		}
	case "server-error":
		cmd = &RpcMethodRespErrorCmd{
			RpcMethodRespCmd: c,
			Done:             true,
			ResultCode:       reqresp.ServerErrCode,
		}
	default:
		return nil, args, fmt.Errorf("unrecognized command: %v", args)
	}
	return cmd, args[1:], nil
}

type RpcMethodRespChunkCmd struct {
	*RpcMethodRespCmd `ask:"-"`
}

func (c *RpcMethodRespChunkCmd) Help() string {
	return "Respond a chunk to a request"
}

func (c *RpcMethodRespChunkCmd) Get(ctx context.Context, args ...string) (cmd interface{}, remaining []string, err error) {
	if len(args) == 0 {
		return nil, nil, errors.New("no subcommand specified")
	}
	switch args[0] {
	case "raw":
		cmd = &RpcMethodRespChunkRawCmd{
			RpcMethodRespChunkCmd: c,
			// if there is one normally only one response chunk, then close it right after by default.
			Done:       c.Method.DefaultResponseChunkCount <= 1,
			ResultCode: reqresp.SuccessCode,
		}
	default:
		return nil, args, fmt.Errorf("unrecognized command: %v", args)
	}
	return cmd, args[1:], nil
}

type RpcMethodRespChunkRawCmd struct {
	*RpcMethodRespChunkCmd `ask:"-"`
	Done                   bool                 `ask:"--done" help:"After writing this chunk, close the response (no more chunks)."`
	ResultCode             reqresp.ResponseCode `ask:"--result-code" help:"Customize the chunk result code. (0 = success, 1 = invalid input, 2 = error, 3+ = undefined)"`
	ReqId                  string               `ask:"<req-id>" help:"the ID of the request to respond to"`
	Data                   BytesHexFlag         `ask:"<data>" help:"chunk bytes (uncompressed, hex-encoded)"`
}

func (c *RpcMethodRespChunkRawCmd) Help() string {
	return "Respond a raw chunk to a request"
}

func (c *RpcMethodRespChunkRawCmd) Run(ctx context.Context, args ...string) error {
	key, req, err := c.checkAndGetReq(c.ReqId)
	if err != nil {
		return err
	}
	if err := req.handler.WriteRawResponseChunk(c.ResultCode, c.Data); err != nil {
		return err
	}
	if c.Done {
		c.Responder.CloseRequest(key)
	}
	return nil
}

type RpcMethodRespErrorCmd struct {
	*RpcMethodRespCmd `ask:"-"`
	Done              bool                 `ask:"--done" help:"After writing this chunk, close the response (no more chunks)."`
	ResultCode        reqresp.ResponseCode `ask:"--result-code" help:"Customize the chunk result code. (0 = success, 1 = invalid input, 2 = error, 3+ = undefined)"`
	ReqId             string               `ask:"<req-id>" help:"the ID of the request to respond to"`
	Text              string               `ask:"<text>" help:"error text data"`
}

func (c *RpcMethodRespErrorCmd) Help() string {
	return "Respond a raw chunk to a request"
}

func (c *RpcMethodRespErrorCmd) Run(ctx context.Context, args ...string) error {
	key, req, err := c.checkAndGetReq(c.ReqId)
	if err != nil {
		return err
	}
	if err := req.handler.WriteErrorChunk(c.ResultCode, c.Text); err != nil {
		return err
	}
	if c.Done {
		c.Responder.CloseRequest(key)
	}
	return nil
}

type RpcMethodCloseCmd struct {
	*RpcMethodCmd `ask:"-"`
	ReqId         string `ask:"<req-id>" help:"the ID of the request to close"`
}

func (c *RpcMethodCloseCmd) Help() string {
	return "Close open requests"
}

func (c *RpcMethodCloseCmd) Run(ctx context.Context, args ...string) error {
	key, req, err := c.checkAndGetReq(c.ReqId)
	if err != nil {
		return err
	}
	c.Responder.CloseRequest(key)
	c.log.WithFields(logrus.Fields{
		"req_id": key,
		"peer":   req.From.String(),
	}).Infof("Closed request.")
	return nil
}
