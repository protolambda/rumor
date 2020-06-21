package gossip

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
)

type GossipBlacklistCmd struct {
	*base.Base
	*GossipState
	PeerID flags.PeerIDFlag `ask:"<peer-id>" help:"The peer to blacklist"`
}

func (c *GossipBlacklistCmd) Help() string {
	return "Blacklist a peer from GossipSub"
}

func (c *GossipBlacklistCmd) Run(ctx context.Context, args ...string) error {
	if c.GossipState.GsNode == nil {
		return NoGossipErr
	}
	c.GossipState.GsNode.BlacklistPeer(c.PeerID.PeerID)
	c.Log.Infof("Blacklisted peer %s", c.PeerID.PeerID.Pretty())
	return nil
}
