package gossip

import (
	"context"
	"fmt"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/protolambda/rumor/control/actor/base"
)

type GossipLeaveCmd struct {
	*base.Base
	*GossipState
	TopicName string `ask:"<topic>" help:"The name of the topic to leave"`
}

func (c *GossipLeaveCmd) Help() string {
	return "Leave a gossip topic."
}

func (c *GossipLeaveCmd) Run(ctx context.Context, args ...string) error {
	if c.GossipState.GsNode == nil {
		return NoGossipErr
	}
	if top, ok := c.GossipState.Topics.Load(c.TopicName); !ok {
		return fmt.Errorf("not on gossip topic %s", c.TopicName)
	} else {
		err := top.(*pubsub.Topic).Close()
		if err != nil {
			return err
		}
		c.GossipState.Topics.Delete(c.TopicName)
		return nil
	}
}
