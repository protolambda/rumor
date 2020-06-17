package peer

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
)

type PeerAddrsCmd struct {
	*base.Base
	PeerID flags.PeerIDFlag `ask:"[peer-id]" help:"The peer to view addresses of, or local peer if omitted."`
}

func (c *PeerAddrsCmd) Help() string {
	return "View known addresses of [peerID]. Defaults to local addresses if no peer id is specified."
}

func (c *PeerAddrsCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	if c.PeerID.PeerID == "" {
		addrs := h.Addrs()
		c.Log.WithField("addrs", addrs).Infof("host addrs")
	} else {
		addrs := h.Peerstore().Addrs(c.PeerID.PeerID)
		c.Log.WithField("addrs", addrs).Infof("addrs for peer %s", c.PeerID.PeerID.Pretty())
	}
	return nil
}
