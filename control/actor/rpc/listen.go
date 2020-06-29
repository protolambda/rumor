package rpc

import (
	"context"
	"encoding/hex"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"time"
)

type RpcMethodListenCmd struct {
	*base.Base
	*RpcMethodData
	Timeout     time.Duration         `ask:"--timeout" help:"Apply timeout of n milliseconds to each stream (complete request <> response time). 0 to Disable timeout"`
	Compression flags.CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
	Raw         bool                  `ask:"--raw" help:"Do not decode the request, look at raw bytes"`
	Drop        bool                  `ask:"--drop" help:"Drop the requests, do not queue for a response."`
	Read        bool                  `ask:"--read" help:"Read the contents of the request."`
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
			c.Log.WithField("req", req).Infof("Received request, dropping it!")
		} else {
			ctx, cancel := context.WithCancel(ctx)
			reqId := c.Responder.AddRequest(&RequestEntry{
				From:    peerId,
				Handler: handler,
				Cancel:  cancel,
			})
			req["req_id"] = reqId

			c.Log.WithField("req", req).Infof("Received request, queued it to respond to!")

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
	c.Log.WithField("started", true).Infof("Opened listener")
	<-ctx.Done()
	h.RemoveStreamHandler(prot)
	return nil
}
