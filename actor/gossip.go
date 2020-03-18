package actor

import (
	"context"
	"encoding/hex"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/protolambda/rumor/gossip"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"path"
	"strings"
	"time"
)

type joinedTopics map[string]*pubsub.Topic

type topicLogger struct {
	name      string
	topicName string
	outPath   string
	close     context.CancelFunc
}

type topicLoggers map[string]*topicLogger

type GossipState struct {
	GsNode       gossip.GossipSub
	CloseGS      context.CancelFunc
	Topics       joinedTopics
	TopicLoggers topicLoggers
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
			state.Topics = make(joinedTopics)
			state.TopicLoggers = make(topicLoggers)
			ctx, cancel := context.WithCancel(r.Ctx)
			var err error
			state.GsNode, err = gossip.NewGossipSub(ctx, log, r)
			if err != nil {
				log.Error(err)
				return
			}
			state.CloseGS = cancel
			log.Info("Started GossipSub")
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop GossipSub",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if noGS(cmd) {
				return
			}
			state.GsNode = nil

			state.CloseGS()
			log.Info("Stopped GossipSub")
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
			topics := make([]string, 0, len(state.Topics))
			for topic := range state.Topics {
				topics = append(topics, topic)
			}
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
			if _, ok := state.Topics[topicName]; ok {
				log.Errorf("already on gossip topic %s", topicName)
				return
			}
			top, err := state.GsNode.JoinTopic(topicName)
			if err != nil {
				log.Error(err)
				return
			}
			state.Topics[topicName] = top
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
			if top, ok := state.Topics[topicName]; !ok {
				log.Errorf("not on gossip topic %s", topicName)
				return
			} else {
				evHandler, err := top.EventHandler()
				if err != nil {
					log.Error(err)
				} else {
					log.Infof("Started listening for peer join/leave events for topic %s", topicName)
					for {
						ev, err := evHandler.NextPeerEvent(r.Ctx)
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
			if top, ok := state.Topics[topicName]; !ok {
				log.Errorf("not on gossip topic %s", topicName)
				return
			} else {
				peers := top.ListPeers()
				log.Infof("%d peers on topic %s", len(peers), topicName)
				for i, p := range peers {
					log.Infof("%4d: %s", i, r.P2PHost.Peerstore().PeerInfo(p).String())
				}
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
			if top, ok := state.Topics[topicName]; !ok {
				log.Errorf("not on gossip topic %s", topicName)
				return
			} else {
				err := top.Close()
				if err != nil {
					log.Error(err)
				}
				delete(state.Topics, topicName)
			}
		},
	})

	logToFile := func(ctx context.Context, loggerName string, top *pubsub.Topic, outPath string) error {
		topicLog := log.WithField("ps_logger", loggerName)
		out, err := os.OpenFile(path.Join(outPath), os.O_WRONLY|os.O_CREATE|os.O_APPEND, os.ModePerm)
		if err != nil {
			return err
		}
		go func() {
			ticker := time.NewTicker(time.Second * 60)
			for {
				select {
				case <-ticker.C:
					if err := out.Sync(); err != nil {
						topicLog.Errorf("Synced %s log with error: %v", outPath, err)
					}
				case <-ctx.Done():
					if err := out.Close(); err != nil {
						topicLog.Errorf("Closed %s log with error: %v", outPath, err)
					}
					return
				}
			}
		}()
		errLogger := gossip.NewErrLoggerChannel(ctx, topicLog, outPath)
		msgLogger := gossip.NewMessageLogger(ctx, out, errLogger)
		return state.GsNode.LogTopic(ctx, loggerName, top, msgLogger, errLogger)
	}

	logCmd := &cobra.Command{
		Use:   "log",
		Short: "Log GossipSub topic messages",
	}

	logCmd.AddCommand(&cobra.Command{
		Use:   "start <name> <topic> <output-file-path>",
		Short: "Log the messages of a gossip topic to a file. 1 hex-encoded message per line. Join a topic first.",
		Args:  cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			if noGS(cmd) {
				return
			}
			loggerName := args[0]
			topicName := args[1]
			outPath := args[2]
			if top, ok := state.Topics[topicName]; !ok {
				log.Errorf("not on gossip topic %s", topicName)
				return
			} else {
				ctx, cancelLogger := context.WithCancel(r.Ctx)
				if err := logToFile(ctx, loggerName, top, outPath); err != nil {
					log.Error(err)
					return
				}
				state.TopicLoggers[loggerName] = &topicLogger{
					name:      loggerName,
					topicName: topicName,
					outPath:   outPath,
					close:     cancelLogger,
				}
			}
		},
	})
	logCmd.AddCommand(&cobra.Command{
		Use:   "stop <name>",
		Short: "Stop logging messages for a logger started with 'log start' earlier",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noGS(cmd) {
				return
			}
			loggerName := args[0]
			if lo, ok := state.TopicLoggers[loggerName]; ok {
				lo.close()
				log.Infof("Closed logger '%s', topic: '%s', output path: '%s'", loggerName, lo.topicName, lo.outPath)
				delete(state.TopicLoggers, loggerName)
			} else {
				log.Infof("Logger '%s' does not exist, cannot stop it.", loggerName)
			}
		},
	})
	logCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List gossip loggers",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if noGS(cmd) {
				return
			}
			log.Infof("%d loggers", len(state.TopicLoggers))
			for name, lo := range state.TopicLoggers {
				log.Infof("%20s: '%s' -> '%s'", name, lo.topicName, lo.outPath)
			}
		},
	})
	cmd.AddCommand(logCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "publish <topic> <message>",
		Short: "Publish a message to the topic. The message should be hex-encoded.",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if noGS(cmd) {
				return
			}
			topicName := args[0]
			if top, ok := state.Topics[topicName]; !ok {
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
				if err := top.Publish(r.Ctx, data); err != nil {
					log.Errorf("failed to publish message, err: %v", err)
					return
				}
			}
		},
	})
	return cmd
}
