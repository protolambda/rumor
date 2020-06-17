package peer

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
)

type PeerProtectCmd struct {
	*base.Base
	PeerID flags.PeerIDFlag `ask:"<peer-id>" help:"The peer to protect with a tag"`
	Tag    string           `ask:"<tag>" help:"Tag to give to the peer"`
}

func (c *PeerProtectCmd) Help() string {
	return "Protect a peer by giving it a tag"
}

func (c *PeerProtectCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	h.ConnManager().Protect(c.PeerID.PeerID, c.Tag)
	c.Log.Infof("protected peer %s as %s", c.PeerID.PeerID.Pretty(), c.Tag)
	return nil
}
