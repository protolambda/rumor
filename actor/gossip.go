package actor

import (
	"context"
	"encoding/hex"
	"github.com/golang/snappy"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/protolambda/rumor/gossip"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"strings"
	"sync"
)

type GossipState struct {
	GsNode  gossip.GossipSub
	CloseGS context.CancelFunc
	// string -> *pubsub.Topic
	Topics sync.Map
}

func (r *Actor) InitGossipCmd(ctx context.Context, log logrus.FieldLogger, state *GossipState) *cobra.Command {
	noGS := func(cmd *cobra.Command) bool {
		if r.NoHost(log) {
			return true
		}
		if state.GsNode == nil {
			log.Error("REPL must have initialized GossipSub. Try 'gossip start'")
			return true
		}
		return false
	}
	cmd := &cobra.Command{
		Use:   "gossip",
		Short: "Manage Libp2p GossipSub",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Start GossipSub",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(log) {
				return
			}
			if state.GsNode != nil {
				log.Error("Already started GossipSub")
				return
			}
			var err error
			state.GsNode, err = gossip.NewGossipSub(r.ActorCtx, r)
			if err != nil {
				log.Error(err)
				return
			}
			log.Info("Started GossipSub")
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List joined gossip topics",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if noGS(cmd) {
				return
			}
			topics := make([]string, 0)
			state.Topics.Range(func(key, value interface{}) bool {
				topics = append(topics, key.(string))
				return false
			})
			log.WithField("topics", topics).Infof("On %d topics.", len(topics))
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "join <topic>",
		Short: "Join a gossip topic. This only sets up the topic, it does not actively find peers. See `gossip log start` and `gossip publish`.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noGS(cmd) {
				return
			}
			topicName := args[0]
			_, ok := state.Topics.Load(topicName)
			if ok {
				log.Errorf("already on gossip topic %s", topicName)
				return
			}
			top, err := state.GsNode.Join(topicName)
			if err != nil {
				log.Error(err)
				return
			}
			state.Topics.Store(topicName, top)
			log.Infof("joined topic %s", topicName)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "events <topic>",
		Short: "Listen for events (not messages) on this topic. Events: 'join=<peer-ID>', 'leave=<peer-ID>'",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noGS(cmd) {
				return
			}
			topicName := args[0]
			if top, ok := state.Topics.Load(topicName); !ok {
				log.Errorf("not on gossip topic %s", topicName)
				return
			} else {
				evHandler, err := top.(*pubsub.Topic).EventHandler()
				if err != nil {
					log.Error(err)
				} else {
					log.Infof("Started listening for peer join/leave events for topic %s", topicName)
					for {
						ev, err := evHandler.NextPeerEvent(ctx)
						if err != nil {
							log.Infof("Stopped listening for peer join/leave events for topic %s", topicName)
							return
						}
						switch ev.Type {
						case pubsub.PeerJoin:
							log.WithField("join", ev.Peer.Pretty()).Infof("peer %s joined topic %s", ev.Peer.Pretty(), topicName)
						case pubsub.PeerLeave:
							log.WithField("leave", ev.Peer.Pretty()).Infof("peer %s left topic %s", ev.Peer.Pretty(), topicName)
						}
					}
				}
			}
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list-peers <topic>",
		Short: "List the peers known for the given topic",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noGS(cmd) {
				return
			}
			topicName := args[0]
			if top, ok := state.Topics.Load(topicName); !ok {
				log.Errorf("not on gossip topic %s", topicName)
				return
			} else {
				peers := top.(*pubsub.Topic).ListPeers()
				log.WithField("peers", peers).Infof("%d peers on topic %s", len(peers), topicName)
			}
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "blacklist <peer-ID>",
		Short: "Blacklist a peer from GossipSub",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noGS(cmd) {
				return
			}
			peerID, err := peer.Decode(args[0])
			if err != nil {
				log.Error(err)
				return
			}
			state.GsNode.BlacklistPeer(peerID)
			log.Infof("Blacklisted peer %s", peerID.Pretty())
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "leave <topic>",
		Short: "Leave a gossip topic.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noGS(cmd) {
				return
			}
			topicName := args[0]
			if top, ok := state.Topics.Load(topicName); !ok {
				log.Errorf("not on gossip topic %s", topicName)
				return
			} else {
				err := top.(*pubsub.Topic).Close()
				if err != nil {
					log.Error(err)
				}
				state.Topics.Delete(topicName)
			}
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "log <topic>",
		Short: "Log the messages of a gossip topic. Messages are hex-encoded. Join a topic first.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noGS(cmd) {
				return
			}
			topicName := args[0]
			if top, ok := state.Topics.Load(topicName); !ok {
				log.Errorf("not on gossip topic %s", topicName)
				return
			} else {
				sub, err := top.(*pubsub.Topic).Subscribe()
				if err != nil {
					log.Errorf("Cannot open subscription on topic %s: %v", topicName, err)
					return
				}
				for {
					msg, err := sub.Next(ctx)
					if err != nil {
						if err == ctx.Err() { // expected quit, context stopped.
							break
						}
						log.Errorf("Gossip subscription on %s encountered error: %v", topicName, err)
						break
					} else {
						var msgData []byte
						if strings.HasSuffix(topicName, "_snappy") {
							msgData, err = snappy.Decode(nil, msg.Data)
							if err != nil {
								log.Errorf("Cannot decode message on %s with snappy: %v", topicName, err)
								return
							}
						} else {
							msgData = msg.Data
						}
						log.WithFields(logrus.Fields{
							"from":      msg.GetFrom().String(),
							"data":      hex.EncodeToString(msgData),
							"signature": hex.EncodeToString(msg.Signature),
							"seq_no":    hex.EncodeToString(msg.Seqno),
						}).Infof("new message on %s", topicName)
					}
				}
				sub.Cancel()
			}
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "publish <topic> <message>",
		Short: "Publish a message to the topic. The message should be hex-encoded.",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if noGS(cmd) {
				return
			}
			topicName := args[0]
			if top, ok := state.Topics.Load(topicName); !ok {
				log.Errorf("not on gossip topic %s", topicName)
				return
			} else {
				hexData := args[1]
				if strings.HasPrefix(hexData, "0x") {
					hexData = hexData[2:]
				}
				data, err := hex.DecodeString(hexData)
				if err != nil {
					log.Errorf("cannot decode message from hex, err: %v, msg: %s", err, hexData)
					return
				}
				if strings.HasSuffix(topicName, "_snappy") {
					data = snappy.Encode(nil, data)
				}
				if err := top.(*pubsub.Topic).Publish(ctx, data); err != nil {
					log.Errorf("failed to publish message, err: %v", err)
					return
				}
			}
		},
	})
	return cmd
}
