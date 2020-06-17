package status

import (
	"context"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"sync"
	"time"
)

type PeerStatusPollCmd struct {
	*base.Base
	Timeout     time.Duration         `ask:"--timeout" help:"request timeout, 0 to disable"`
	Interval    time.Duration         `ask:"--interval" help:"interval to request status of peers on"`
	Compression flags.CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
}

func (c *PeerStatusPollCmd) Help() string {
	return "Fetch status of all connected peers, repeatedly on the given interval."
}

func (c *PeerStatusPollCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	for {
		start := time.Now()
		var wg sync.WaitGroup
		for _, p := range h.Network().Peers() {
			// TODO: maybe filter peers that cannot answer status requests?
			wg.Add(1)
			go func(peerID peer.ID) {
				if err := c.fetch(ctx, c.Timeout, peerID, c.Compression.Compression); err != nil {
					c.Log.Warn(err)
				}
				wg.Done()
			}(p)
		}
		wg.Wait()
		pollStepDuration := time.Since(start)
		if pollStepDuration < c.Interval {
			time.Sleep(c.Interval - pollStepDuration)
		}
		select {
		case <-ctx.Done():
			return nil
		default:
			// next interval
		}
	}
}
