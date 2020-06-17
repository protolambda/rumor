package gossip

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/golang/snappy"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/sirupsen/logrus"
	"strings"
)

type GossipLogCmd struct {
	*base.Base
	TopicName string `ask:"<topic>" help:"The name of the topic to log messages of"`
}

func (c *GossipLogCmd) Help() string {
	return "Log the messages of a gossip topic. Messages are hex-encoded. Join a topic first."
}

func (c *GossipLogCmd) Run(ctx context.Context, args ...string) error {
	if c.GossipState.GsNode == nil {
		return NoGossipErr
	}
	if top, ok := c.GossipState.Topics.Load(c.TopicName); !ok {
		return fmt.Errorf("not on gossip topic %s", c.TopicName)
	} else {
		sub, err := top.(*pubsub.Topic).Subscribe()
		if err != nil {
			return fmt.Errorf("Cannot open subscription on topic %s: %v", c.TopicName, err)
		}
		defer sub.Cancel()
		for {
			msg, err := sub.Next(ctx)
			if err != nil {
				if err == ctx.Err() { // expected quit, context stopped.
					break
				}
				return fmt.Errorf("Gossip subscription on %s encountered error: %v", c.TopicName, err)
			} else {
				var msgData []byte
				if strings.HasSuffix(c.TopicName, "_snappy") {
					msgData, err = snappy.Decode(nil, msg.Data)
					if err != nil {
						return fmt.Errorf("Cannot decode message on %s with snappy: %v", c.TopicName, err)
					}
				} else {
					msgData = msg.Data
				}
				c.Log.WithFields(logrus.Fields{
					"from":      msg.GetFrom().String(),
					"data":      hex.EncodeToString(msgData),
					"signature": hex.EncodeToString(msg.Signature),
					"seq_no":    hex.EncodeToString(msg.Seqno),
				}).Infof("new message on %s", c.TopicName)
			}
		}
		return nil
	}
}
