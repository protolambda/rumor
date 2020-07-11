package metadata

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

type PeerMetadataServeCmd struct {
	*base.Base
	*PeerMetadataState
	Timeout     time.Duration         `ask:"--timeout" help:"Apply timeout of n milliseconds to each stream (complete request <> response time). 0 to Disable timeout"`
	Compression flags.CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
}

func (c *PeerMetadataServeCmd) Help() string {
	return "Serve incoming metadata requests"
}

func (c *PeerMetadataServeCmd) Default() {
	c.Timeout = 10 * time.Second
	c.Compression = flags.CompressionFlag{Compression: reqresp.SnappyCompression{}}
}

func (c *PeerMetadataServeCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	bgCtx, bgCancel := context.WithCancel(context.Background())
	sCtxFn := func() context.Context {
		if c.Timeout == 0 {
			return bgCtx
		}
		reqCtx, _ := context.WithTimeout(bgCtx, c.Timeout)
		return reqCtx
	}
	comp := c.Compression.Compression
	listenReq := func(ctx context.Context, peerId peer.ID, handler reqresp.ChunkedRequestHandler) {
		f := map[string]interface{}{
			"from": peerId.String(),
		}
		// Metadata requests have no data, but we do need to check the general format.
		err := handler.ReadRequest(nil)
		if err != nil {
			f["input_err"] = err.Error()
			_ = handler.WriteErrorChunk(reqresp.InvalidReqCode, "could not parse metadata request")
			c.Log.WithFields(f).Warnf("failed to read metadata request: %v", err)
		} else {
			if err := handler.WriteResponseChunk(reqresp.SuccessCode, &c.PeerMetadataState.Local); err != nil {
				c.Log.WithFields(f).Warnf("failed to respond to metadata request: %v", err)
			} else {
				c.Log.WithFields(f).Info("handled metadata request")
			}
		}
	}
	m := methods.MetaDataRPCv1
	streamHandler := m.MakeStreamHandler(sCtxFn, comp, listenReq)
	prot := m.Protocol
	if comp != nil {
		prot += protocol.ID("_" + comp.Name())
	}
	h.SetStreamHandler(prot, streamHandler)
	c.Log.WithField("started", true).Info("Started serving metadata")

	c.Control.RegisterStop(func(ctx context.Context) error {
		bgCancel()
		h.RemoveStreamHandler(prot)
		c.Log.Infof("Stopped serving metadata")
		return nil
	})
	return nil
}
