package gossip

import (
	"context"
	"errors"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/gossip"
)

type GossipStartCmd struct {
	*base.Base
	*GossipState
}

func (c *GossipStartCmd) Help() string {
	return "Start GossipSub"
}

func (c *GossipStartCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	if c.GossipState.GsNode != nil {
		return errors.New("Already started GossipSub")
	}
	c.GossipState.GsNode, err = gossip.NewGossipSub(c.ActorContext, h)
	if err != nil {
		return err
	}
	c.Log.Info("Started GossipSub")
	return nil
}
