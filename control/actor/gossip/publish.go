package gossip

import (
	"context"
	"fmt"
	"github.com/golang/snappy"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/protolambda/rumor/control/actor/base"
	"strings"
)

type GossipPublishCmd struct {
	*base.Base
	*GossipState
	TopicName string `ask:"<topic>" help:"The name of the topic to publish to"`
	Message   []byte `ask:"<message>" help:"The uncompressed message bytes, hex-encoded"`
}

func (c *GossipPublishCmd) Help() string {
	return "Publish a message to the topic. The message should be hex-encoded."
}

func (c *GossipPublishCmd) Run(ctx context.Context, args ...string) error {
	if c.GossipState.GsNode == nil {
		return NoGossipErr
	}
	if top, ok := c.GossipState.Topics.Load(c.TopicName); !ok {
		return fmt.Errorf("not on gossip topic %s", c.TopicName)
	} else {
		data := c.Message
		if strings.HasSuffix(c.TopicName, "_snappy") {
			data = snappy.Encode(nil, data)
		}
		if err := top.(*pubsub.Topic).Publish(ctx, data); err != nil {
			return fmt.Errorf("failed to publish message, err: %v", err)
		}
		return nil
	}
}
