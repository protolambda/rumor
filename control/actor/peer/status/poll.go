package status

import (
	"context"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

type PeerStatusPollCmd struct {
	*base.Base
	*PeerStatusState
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

		// apply timeout to each poll target in this round
		reqCtx := ctx
		if c.Timeout != 0 {
			reqCtx, _ = context.WithTimeout(reqCtx, c.Timeout)
		}

		for _, p := range h.Network().Peers() {
			// TODO: maybe filter peers that cannot answer status requests?
			wg.Add(1)
			go func(peerID peer.ID) {
				code, msg, stat, err := c.fetch(h.NewStream, reqCtx, peerID, c.Compression.Compression)
				if err != nil {
					c.Log.Warn(err)
				} else {
					if code == reqresp.SuccessCode {
						c.Log.WithFields(logrus.Fields{
							"code": code,
							"status": stat.Data(),
						}).Debug("status poll success")
					} else {
						c.Log.WithFields(logrus.Fields{
							"code": code,
							"msg": msg,
						}).Debug("status poll non-success")
					}
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
