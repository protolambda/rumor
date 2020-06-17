package peer

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
)

type PeerDisconnectCmd struct {
	*base.Base
	PeerID flags.PeerIDFlag `ask:"<peer-id>" help:"The peer to close all connections of"`
}

func (c *PeerDisconnectCmd) Help() string {
	return "Close all open connections with the given peer"
}

func (c *PeerDisconnectCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	conns := h.Network().ConnsToPeer(c.PeerID.PeerID)
	for _, conn := range conns {
		if err := conn.Close(); err != nil {
			c.Log.Infof("error during disconnect of peer %s (%s)",
				c.PeerID.PeerID.Pretty(), conn.RemoteMultiaddr().String())
		}
	}
	c.Log.Infof("disconnected peer %s", c.PeerID.PeerID.Pretty())
	return nil
}
