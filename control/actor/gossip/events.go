package gossip

import (
	"context"
	"fmt"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/protolambda/rumor/control/actor/base"
)

type GossipEventsCmd struct {
	*base.Base
	TopicName string `ask:"<topic>" help:"The name of the topic to track events of"`
}

func (c *GossipEventsCmd) Help() string {
	return "Listen for events (not messages) on this topic. Events: 'join=<peer-ID>', 'leave=<peer-ID>'"
}

func (c *GossipEventsCmd) Run(ctx context.Context, args ...string) error {
	if c.GossipState.GsNode == nil {
		return NoGossipErr
	}
	top, ok := c.GossipState.Topics.Load(c.TopicName)
	if !ok {
		return fmt.Errorf("not on gossip topic %s", c.TopicName)
	}
	evHandler, err := top.(*pubsub.Topic).EventHandler()
	if err != nil {
		return err
	} else {
		c.Log.Infof("Started listening for peer join/leave events for topic %s", c.TopicName)
		for {
			ev, err := evHandler.NextPeerEvent(ctx)
			if err != nil {
				c.Log.Infof("Stopped listening for peer join/leave events for topic %s", c.TopicName)
				return nil
			}
			switch ev.Type {
			case pubsub.PeerJoin:
				c.Log.WithField("join", ev.Peer.Pretty()).Infof("peer %s joined topic %s", ev.Peer.Pretty(), c.TopicName)
			case pubsub.PeerLeave:
				c.Log.WithField("leave", ev.Peer.Pretty()).Infof("peer %s left topic %s", ev.Peer.Pretty(), c.TopicName)
			}
		}
	}
}
