package status

import (
	"context"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"time"
)

type PeerStatusServeCmd struct {
	*base.Base
	*PeerStatusState

	// TODO set default
	Timeout     time.Duration         `ask:"--timeout" help:"Apply timeout of n milliseconds to each stream (complete request <> response time). 0 to Disable timeout"`
	Compression flags.CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
}

func (c *PeerStatusServeCmd) Help() string {
	return "Serve incoming status requests"
}

func (c *PeerStatusServeCmd) Run(ctx context.Context, args ...string) error {
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
	comp := c.Compression.Compression
	listenReq := func(ctx context.Context, peerId peer.ID, handler reqresp.ChunkedRequestHandler) {
		f := map[string]interface{}{
			"from": peerId.String(),
		}
		var reqStatus methods.Status
		err := handler.ReadRequest(&reqStatus)
		if err != nil {
			f["input_err"] = err.Error()
			_ = handler.WriteErrorChunk(reqresp.InvalidReqCode, "could not parse status request")
			c.Log.WithFields(f).Warnf("failed to read status request: %v", err)
		} else {
			f["data"] = reqStatus
			c.OnStatus(peerId, &reqStatus)

			if err := handler.WriteResponseChunk(reqresp.SuccessCode, &c.PeerStatusState.Local); err != nil {
				c.Log.WithFields(f).Warnf("failed to respond to status request: %v", err)
			} else {
				c.Log.WithFields(f).Info("handled status request")
			}
		}
	}
	m := methods.StatusRPCv1
	streamHandler := m.MakeStreamHandler(sCtxFn, comp, listenReq)
	prot := m.Protocol
	if comp != nil {
		prot += protocol.ID("_" + comp.Name())
	}
	h.SetStreamHandler(prot, streamHandler)
	c.Log.WithField("started", true).Info("Opened listener")
	<-ctx.Done()
	return nil
}
