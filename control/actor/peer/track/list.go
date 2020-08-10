package track

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/track"
)

type PeerTrackListCmd struct {
	*base.Base
	Store track.ExtendedPeerstore
}

func (c *PeerTrackListCmd) Help() string {
	return "List active peerstore tracker tees"
}

func (c *PeerTrackListCmd) Run(ctx context.Context, args ...string) error {
	tees := make([]string, 0)
	for _, t := range c.Store.ListTees() {
		tees = append(tees, t.String())
	}
	c.Log.WithField("tees", tees).Infof("Currently %d active tees", len(tees))
	return nil
}
