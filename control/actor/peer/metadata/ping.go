package metadata

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/sirupsen/logrus"
	"time"
)

type PeerMetadataPingCmd struct {
	*base.Base
	*PeerMetadataState
	Store         track.ExtendedPeerstore
	Timeout       time.Duration         `ask:"--timeout" help:"request timeout for ping, 0 to disable"`
	Compression   flags.CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
	Update        bool                  `ask:"--update" help:"If the seq nr pong is higher than known, request metadata"`
	ForceUpdate   bool                  `ask:"--force-update" help:"Force a metadata request, even if the ping results in an already known pong seq nr"`
	UpdateTimeout time.Duration         `ask:"--update-timeout" help:"If updating, use this timeout for the update request, 0 to disable."`
	PeerID        flags.PeerIDFlag      `ask:"<peer-id>" help:"Peer to fetch metadata from."`
	MaxTries      uint64                `ask:"--max-tries" help:"How many times an update should be attempted after learning about a pong"`
}

func (c *PeerMetadataPingCmd) Help() string {
	return "Ping a connected peer to get their seq nr, optionally req metadata after."
}

func (c *PeerMetadataPingCmd) Default() {
	c.Timeout = 10 * time.Second
	c.UpdateTimeout = 10 * time.Second
	c.Compression = flags.CompressionFlag{Compression: reqresp.SnappyCompression{}}
	c.Update = true
	c.MaxTries = 20
}

func (c *PeerMetadataPingCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	reqCtx := ctx
	if c.Timeout != 0 {
		reqCtx, _ = context.WithTimeout(reqCtx, c.Timeout)
	}
	peerID := c.PeerID.PeerID

	var startTime time.Time
	newStream := reqresp.NewStreamFn(func(ctx context.Context, peerId peer.ID, protocolId ...protocol.ID) (network.Stream, error) {
		startTime = time.Now()
		return h.NewStream(ctx, peerId, protocolId...)
	})
	code, msg, pong, err := c.ping(newStream, reqCtx, peerID, c.Compression.Compression)
	if err != nil {
		return fmt.Errorf("failed to ping: %v", err)
	} else {
		c.Store.RecordLatency(peerID, time.Since(startTime))
		if code == reqresp.SuccessCode {
			c.Log.WithFields(logrus.Fields{
				"code": code,
				"pong": pong,
			}).Debug("ping request success")
			c.Store.RegisterSeqClaim(peerID, beacon.SeqNr(pong))
		} else {
			c.Log.WithFields(logrus.Fields{
				"code": code,
				"msg":  msg,
			}).Debug("ping request non-success")
		}
	}

	updating := c.ForceUpdate
	if !updating && c.Update {
		current := c.Store.Metadata(peerID)
		if current == nil || current.SeqNumber < beacon.SeqNr(pong) {
			fetches := c.Store.RegisterMetaFetch(peerID)
			updating = fetches <= c.MaxTries
		}
	}
	if updating {
		c.Log.WithField("pong", pong).Debug("Got pong, following up with metadata request")
		updateCtx := ctx
		if c.UpdateTimeout != 0 {
			updateCtx, _ = context.WithTimeout(updateCtx, c.UpdateTimeout)
		}
		code, msg, metadata, err := c.fetch(c.Store, h.NewStream, updateCtx, peerID, c.Compression.Compression)
		if err != nil {
			return fmt.Errorf("failed to fetch metadata upon pong: %v", err)
		} else {
			if code == reqresp.SuccessCode {
				c.Log.WithFields(logrus.Fields{
					"code":     code,
					"metadata": metadata.Data(),
				}).Debug("metadata request upon pong success")
			} else {
				c.Log.WithFields(logrus.Fields{
					"code": code,
					"msg":  msg,
				}).Debug("metadata request upon pong non-success")
			}
		}
	}
	return nil
}
