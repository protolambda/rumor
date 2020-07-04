package peerstore

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/track"
)

type SwitchCmd struct {
	*base.Base

	GlobalPeerstores track.Peerstores
	CurrentPeerstore track.DynamicPeerstore

	ID track.PeerstoreID `ask:"<id>" help:"ID of the peerstore to switch to"`
}

func (c *SwitchCmd) Help() string {
	return "Switch peerstore"
}

func (c *SwitchCmd) Run(ctx context.Context, args ...string) error {
	ep, ok := c.GlobalPeerstores.Find(c.ID)
	if !ok {
		return fmt.Errorf("peerstore %s does not exist", c.ID)
	}
	h, err := c.Host()
	var retainId peer.ID
	if err == nil {
		retainId = h.ID()
	}
	c.CurrentPeerstore.Switch(retainId, c.ID, ep)
	c.Log.WithField("store", c.ID).Info("Switched peerstore")
	return nil
}
