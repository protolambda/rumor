package actor

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/golang/snappy"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/protolambda/rumor/p2p/gossip"
	"github.com/sirupsen/logrus"
	"strings"
	"sync"
)

type GossipState struct {
	GsNode  gossip.GossipSub
	CloseGS context.CancelFunc
	// string -> *pubsub.Topic
	Topics sync.Map
}

type GossipCmd struct {
	*Actor       `ask:"-"`
	log          logrus.FieldLogger
	*GossipState `ask:"-"`
}

func (c *GossipCmd) Get(ctx context.Context, args ...string) (cmd interface{}, remaining []string, err error) {
	if len(args) == 0 {
		return nil, nil, errors.New("no subcommand specified")
	}
	switch args[0] {
	case "start":
		cmd = &GossipStartCmd{GossipCmd: c}

	default:
		return nil, args, fmt.Errorf("unrecognized command: %v", args)
	}
	return cmd, args[1:], nil
}

func (c *GossipCmd) Help() string {
	return "Manage Libp2p GossipSub" // TODO list subcommands
}

type GossipStartCmd struct {
	*GossipCmd `ask:"-"`
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
	c.GossipState.GsNode, err = gossip.NewGossipSub(c.ActorCtx, h)
	if err != nil {
		return err
	}
	c.log.Info("Started GossipSub")
	return nil
}

var NoGossipErr = errors.New("Must start gossip-sub first. Try 'gossip start'")

type GossipListCmd struct {
	*GossipCmd `ask:"-"`
}

func (c *GossipListCmd) Help() string {
	return "List joined gossip topics"
}

func (c *GossipListCmd) Run(ctx context.Context, args ...string) error {
	if c.GossipState.GsNode == nil {
		return NoGossipErr
	}
	topics := make([]string, 0)
	c.GossipState.Topics.Range(func(key, value interface{}) bool {
		topics = append(topics, key.(string))
		return false
	})
	c.log.WithField("topics", topics).Infof("On %d topics.", len(topics))
	return nil
}

type GossipJoinCmd struct {
	*GossipCmd `ask:"-"`
	TopicName  string `ask:"<topic>" help:"The name of the topic to join"`
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
	c.log.Infof("joined topic %s", c.TopicName)
	return nil
}

type GossipEventsCmd struct {
	*GossipCmd `ask:"-"`
	TopicName  string `ask:"<topic>" help:"The name of the topic to track events of"`
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
		c.log.Infof("Started listening for peer join/leave events for topic %s", c.TopicName)
		for {
			ev, err := evHandler.NextPeerEvent(ctx)
			if err != nil {
				c.log.Infof("Stopped listening for peer join/leave events for topic %s", c.TopicName)
				return nil
			}
			switch ev.Type {
			case pubsub.PeerJoin:
				c.log.WithField("join", ev.Peer.Pretty()).Infof("peer %s joined topic %s", ev.Peer.Pretty(), c.TopicName)
			case pubsub.PeerLeave:
				c.log.WithField("leave", ev.Peer.Pretty()).Infof("peer %s left topic %s", ev.Peer.Pretty(), c.TopicName)
			}
		}
	}
}

type GossipListPeersCmd struct {
	*GossipCmd `ask:"-"`
	TopicName  string `ask:"<topic>" help:"The name of the topic to list peers of"`
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
		c.log.WithField("peers", peers).Infof("%d peers on topic %s", len(peers), c.TopicName)
		return nil
	}
}

type GossipBlacklistCmd struct {
	*GossipCmd `ask:"-"`
	PeerID     PeerIDFlag `ask:"<peer-id>" help:"The peer to blacklist"`
}

func (c *GossipBlacklistCmd) Help() string {
	return "Blacklist a peer from GossipSub"
}

func (c *GossipBlacklistCmd) Run(ctx context.Context, args ...string) error {
	if c.GossipState.GsNode == nil {
		return NoGossipErr
	}
	c.GossipState.GsNode.BlacklistPeer(c.PeerID.PeerID)
	c.log.Infof("Blacklisted peer %s", c.PeerID.PeerID.Pretty())
	return nil
}

type GossipLeaveCmd struct {
	*GossipCmd `ask:"-"`
	TopicName  string `ask:"<topic>" help:"The name of the topic to leave"`
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

type GossipLogCmd struct {
	*GossipCmd `ask:"-"`
	TopicName  string `ask:"<topic>" help:"The name of the topic to log messages of"`
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
				c.log.WithFields(logrus.Fields{
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

type GossipPublishCmd struct {
	*GossipCmd `ask:"-"`
	TopicName  string       `ask:"<topic>" help:"The name of the topic to publish to"`
	Message    BytesHexFlag `ask:"<message>" help:"The uncompressed message bytes, hex-encoded"`
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
