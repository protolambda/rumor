package peer

import (
	"context"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"time"
)

type PeerConnectCmd struct {
	*base.Base
	Timeout  time.Duration    `ask:"--timeout" help:"connection timeout, 0 to disable"`
	Addr     flags.FlexibleAddrFlag `ask:"<addr>" help:"ENR, enode or multi address to connect to"`
	Tag      string           `ask:"[tag]" help:"Optionally tag the peer upon connection, e.g. tag 'bootnode'"`
}

func (c *PeerConnectCmd) Help() string {
	return "Connect to peer."
}

func (c *PeerConnectCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	addrInfo, err := peer.AddrInfoFromP2pAddr(c.Addr.MultiAddr)
	if err != nil {
		return err
	}
	if c.Timeout != 0 {
		ctx, _ = context.WithTimeout(ctx, c.Timeout)
	}
	if err := h.Connect(ctx, *addrInfo); err != nil {
		return err
	}
	c.Log.WithField("peer_id", addrInfo.ID.Pretty()).Infof("connected to peer")
	if c.Tag != "" {
		h.ConnManager().Protect(addrInfo.ID, c.Tag)
		c.Log.Infof("tagged peer %s as %s", addrInfo.ID.Pretty(), c.Tag)
	}
	return nil
}
