package host

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
)

type HostViewCmd struct {
	*base.Base
	base.WithEnrNode
}

func (c *HostViewCmd) Help() string {
	return "View local peer ID, listening addresses, etc."
}

func (c *HostViewCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	c.Log.WithField("peer_id", h.ID()).Info("Peer ID")
	for i, a := range h.Addrs() {
		c.Log.WithField("addr", a.String()+"/p2p/"+h.ID().String()).Infof("Listening address %d", i)
	}
	node, ok := c.GetNode()
	if ok {
		c.Log.WithField("enr", node.String()).Info("ENR")
	}
	return nil
}
