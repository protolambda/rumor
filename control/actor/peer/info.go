package peer

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/track"
)

type PeerInfoCmd struct {
	*base.Base
	Store track.ExtendedPeerstore

	PeerID flags.PeerIDFlag `ask:"<peer-ID>" help:"peer ID"`
}

func (c *PeerInfoCmd) Help() string {
	return "Read info about a specific peer from the peerstore."
}

func (c *PeerInfoCmd) Run(ctx context.Context, args ...string) error {
	info := c.Store.GetAllData(c.PeerID.PeerID)
	c.Log.WithField("info", info).Infof("peer info")
	return nil
}
