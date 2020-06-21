package gossip

import (
	"context"
	"fmt"
	"github.com/protolambda/rumor/control/actor/base"
)

type GossipJoinCmd struct {
	*base.Base
	*GossipState
	TopicName string `ask:"<topic>" help:"The name of the topic to join"`
}

func (c *GossipJoinCmd) Help() string {
	return "Join a gossip topic. This only sets up the topic, it does not actively find peers. See `gossip log start` and `gossip publish`."
}

func (c *GossipJoinCmd) Run(ctx context.Context, args ...string) error {
	if c.GossipState.GsNode == nil {
		return NoGossipErr
	}
	_, ok := c.GossipState.Topics.Load(c.TopicName)
	if ok {
		return fmt.Errorf("already on gossip topic %s", c.TopicName)
	}
	top, err := c.GossipState.GsNode.Join(c.TopicName)
	if err != nil {
		return err
	}
	c.GossipState.Topics.Store(c.TopicName, top)
	c.Log.Infof("joined topic %s", c.TopicName)
	return nil
}
