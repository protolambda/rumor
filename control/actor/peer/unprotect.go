package peer

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
)

type PeerUnprotectCmd struct {
	*base.Base
	PeerID flags.PeerIDFlag `ask:"<peer-id>" help:"The peer to un-protect with a tag"`
	Tag    string           `ask:"<tag>" help:"Tag to remove from the peer"`
}

func (c *PeerUnprotectCmd) Help() string {
	return "Unprotect a peer by removing a tag"
}

func (c *PeerUnprotectCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	h.ConnManager().Unprotect(c.PeerID.PeerID, c.Tag)
	c.Log.Infof("un-protected peer %s as %s", c.PeerID.PeerID.Pretty(), c.Tag)
	return nil
}
