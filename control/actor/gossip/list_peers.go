package gossip

import (
	"context"
	"fmt"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/protolambda/rumor/control/actor/base"
)

type GossipListPeersCmd struct {
	*base.Base
	*GossipState
	TopicName string `ask:"<topic>" help:"The name of the topic to list peers of"`
}

func (c *GossipListPeersCmd) Help() string {
	return "List the peers known for the given topic"
}

func (c *GossipListPeersCmd) Run(ctx context.Context, args ...string) error {
	if c.GossipState.GsNode == nil {
		return NoGossipErr
	}
	if top, ok := c.GossipState.Topics.Load(c.TopicName); !ok {
		return fmt.Errorf("not on gossip topic %s", c.TopicName)
	} else {
		peers := top.(*pubsub.Topic).ListPeers()
		c.Log.WithField("peers", peers).Infof("%d peers on topic %s", len(peers), c.TopicName)
		return nil
	}
}
