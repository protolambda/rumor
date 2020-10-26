package metadata

import (
	"context"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/protolambda/rumor/p2p/track"
	"sync"
	"time"
)

type PeerMetadataPollCmd struct {
	*base.Base
	*PeerMetadataState
	Store         track.ExtendedPeerstore
	Timeout       time.Duration         `ask:"--timeout" help:"request timeout for ping, 0 to disable."`
	Interval      time.Duration         `ask:"--interval" help:"interval to send pings to peers on, applied as timeout to a round of work"`
	Update        bool                  `ask:"--update" help:"If the seq nr pong is higher than known, request metadata"`
	ForceUpdate   bool                  `ask:"--force-update" help:"Force a metadata request, even if the ping results in an already known pong seq nr"`
	UpdateTimeout time.Duration         `ask:"--update-timeout" help:"If updating, use this timeout for the update request, 0 to disable."`
	MaxTries      uint64                `ask:"--max-tries" help:"How many times an update should be attempted after learning about a pong"`
	Compression   flags.CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
}

func (c *PeerMetadataPollCmd) Default() {
	c.Timeout = 10 * time.Second
	c.UpdateTimeout = 10 * time.Second
	c.Interval = 20 * time.Second
	c.Compression = flags.CompressionFlag{Compression: reqresp.SnappyCompression{}}
	c.Update = true
	c.MaxTries = 20
}

func (c *PeerMetadataPollCmd) Help() string {
	return "Ping all connected peers, repeatedly on the given interval. Optionally update if seq nr is new."
}

func (c *PeerMetadataPollCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}

	stopping := false
	bgCtx, bgCancel := context.WithCancel(context.Background())
	go func() {
		for {
			if stopping {
				return
			}
			start := time.Now()
			var wg sync.WaitGroup

			// apply timeout to each poll target in this round
			reqCtx, _ := context.WithTimeout(bgCtx, c.Interval)

			for _, p := range h.Network().Peers() {
				wg.Add(1)
				go func(peerID peer.ID) {
					pingCmd := &PeerMetadataPingCmd{
						Base:              c.Base,
						PeerMetadataState: c.PeerMetadataState,
						Store:             c.Store,
						Timeout:           c.Timeout,
						Compression:       c.Compression,
						Update:            c.Update,
						ForceUpdate:       c.ForceUpdate,
						UpdateTimeout:     c.UpdateTimeout,
						MaxTries:          c.MaxTries,
						PeerID:            flags.PeerIDFlag{PeerID: peerID},
					}
					if err := pingCmd.Run(reqCtx); err != nil {
						c.Log.WithField("peer", peerID.String()).WithError(err).Warn("failed to poll peer")
					}

					wg.Done()
				}(p)
			}
			wg.Wait()
			pollStepDuration := time.Since(start)
			if pollStepDuration < c.Interval {
				time.Sleep(c.Interval - pollStepDuration)
			}
		}
	}()
	c.Control.RegisterStop(func(ctx context.Context) error {
		stopping = true
		bgCancel()
		c.Log.Infof("Stopped polling")
		return nil
	})

	return nil
}
