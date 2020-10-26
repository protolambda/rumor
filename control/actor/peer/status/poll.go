package status

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

type PeerStatusPollCmd struct {
	*base.Base
	*PeerStatusState
	Book        track.StatusBook
	Timeout     time.Duration         `ask:"--timeout" help:"request timeout, 0 to disable."`
	Interval    time.Duration         `ask:"--interval" help:"interval to request status of peers on, applied as timeout to a round of work"`
	Compression flags.CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
}

func (c *PeerStatusPollCmd) Help() string {
	return "Fetch status of all connected peers, repeatedly on the given interval."
}

func (c *PeerStatusPollCmd) Default() {
	c.Timeout = 5 * time.Second
	c.Interval = 12 * time.Second
	c.Compression = flags.CompressionFlag{Compression: reqresp.SnappyCompression{}}
}

func (c *PeerStatusPollCmd) Run(ctx context.Context, args ...string) error {
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
					statusCmd := &PeerStatusReqCmd{
						Base:            c.Base,
						PeerStatusState: c.PeerStatusState,
						Book:            c.Book,
						Timeout:         c.Timeout,
						Compression:     c.Compression,
						PeerID:          flags.PeerIDFlag{PeerID: peerID},
					}
					if err := statusCmd.Run(reqCtx); err != nil {
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
