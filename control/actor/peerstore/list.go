package peerstore

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/track"
)

type ListCmd struct {
	*base.Base

	GlobalPeerstores track.Peerstores
	CurrentPeerstore track.DynamicPeerstore
}

func (c *ListCmd) Help() string {
	return "List peerstores and identify current peerstore"
}

func (c *ListCmd) Run(ctx context.Context, args ...string) error {
	stores := c.GlobalPeerstores.List()
	c.Log.WithField("stores", stores).Infof("Got %d peerstores", len(stores))
	if c.CurrentPeerstore.Initialized() {
		c.Log.WithField("current", c.CurrentPeerstore.PeerstoreID()).Info("Current peerstore")
	} else {
		c.Log.WithField("current", "").Info("No current peerstore")
	}
	return nil
}
