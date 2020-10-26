package metadata

import (
	"context"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/protolambda/zrnt/eth2/beacon"
	"time"
)

type PeerMetadataPongCmd struct {
	*base.Base
	*PeerMetadataState
	Book          track.MetadataBook
	Timeout       time.Duration         `ask:"--timeout" help:"Apply timeout of n milliseconds to each stream (complete request <> response time). 0 to Disable timeout"`
	Compression   flags.CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
	Update        bool                  `ask:"--update" help:"If the seq nr ping is higher than known, request metadata"`
	ForceUpdate   bool                  `ask:"--force-update" help:"Force a metadata request, even if the ping is an already past seq nr"`
	UpdateTimeout time.Duration         `ask:"--update-timeout" help:"If updating, use this timeout for the update request, 0 to disable. Independent of the ping handling timeout."`
	MaxTries      uint64                `ask:"--max-tries" help:"How many times an update should be attempted after learning about a ping"`
}

func (c *PeerMetadataPongCmd) Help() string {
	return "Serve incoming ping requests: pong back, and optionally request metadata back"
}

func (c *PeerMetadataPongCmd) Default() {
	c.Timeout = 10 * time.Second
	c.UpdateTimeout = 10 * time.Second
	c.Compression = flags.CompressionFlag{Compression: reqresp.SnappyCompression{}}
	c.Update = true
	c.MaxTries = 20
}

func (c *PeerMetadataPongCmd) Run(ctx context.Context, args ...string) error {
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
	listenReq := func(_ context.Context, peerId peer.ID, handler reqresp.ChunkedRequestHandler) {
		f := map[string]interface{}{
			"from": peerId.String(),
		}
		var ping beacon.Ping
		err := handler.ReadRequest(&ping)
		if err != nil {
			f["input_err"] = err.Error()
			_ = handler.WriteErrorChunk(reqresp.InvalidReqCode, "could not parse ping request")
			c.Log.WithFields(f).Warnf("failed to read ping request: %v", err)
		} else {
			pong := beacon.Pong(c.PeerMetadataState.Local.SeqNumber)
			if err := handler.WriteResponseChunk(reqresp.SuccessCode, &pong); err != nil {
				c.Log.WithFields(f).Warnf("failed to respond to ping request: %v", err)
			} else {
				c.Log.WithFields(f).Info("handled ping request")
			}
			c.Book.RegisterSeqClaim(peerId, beacon.SeqNr(ping))
			updating := c.ForceUpdate
			if !updating && c.Update {
				current := c.Book.Metadata(peerId)
				if current == nil || current.SeqNumber < beacon.SeqNr(ping) {
					fetches := c.Book.RegisterMetaFetch(peerId)
					updating = fetches <= c.MaxTries
				}
			}
			if updating {
				req := &PeerMetadataReqCmd{
					Base:              c.Base,
					PeerMetadataState: c.PeerMetadataState,
					Book:              c.Book,
					Timeout:           c.UpdateTimeout,
					Compression:       c.Compression,
					PeerID:            flags.PeerIDFlag{PeerID: peerId},
				}
				go func() {
					// use command context, update timeout is applied independently from the ctx of the ping.
					if err := req.Run(ctx); err != nil {
						c.Log.WithFields(f).Warnf("failed to request metadata as follow up to ping request: %v", err)
					}
				}()
			}
		}
	}
	m := methods.PingRPCv1
	streamHandler := m.MakeStreamHandler(sCtxFn, comp, listenReq)
	prot := m.Protocol
	if comp != nil {
		prot += protocol.ID("_" + comp.Name())
	}
	h.SetStreamHandler(prot, streamHandler)
	c.Log.WithField("started", true).Info("Started serving pongs")

	c.Control.RegisterStop(func(ctx context.Context) error {
		bgCancel()
		h.RemoveStreamHandler(prot)
		c.Log.Infof("Stopped serving pongs")
		return nil
	})
	return nil
}
