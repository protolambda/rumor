package actor

import (
	"context"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/protolambda/rumor/peering/kad"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"time"
)

type KadState struct {
	KadNode  kad.Kademlia
	CloseKad context.CancelFunc
}

func (r *Actor) InitKadCmd(ctx context.Context, log logrus.FieldLogger, state *KadState) *cobra.Command {
	noKad := func(cmd *cobra.Command) bool {
		if r.NoHost(log) {
			return true
		}
		if state.KadNode == nil {
			log.Error("REPL must have initialized Kademlia DHT. Try 'kad start'")
			return true
		}
		return false
	}
	cmd := &cobra.Command{
		Use:   "kad",
		Short: "Manage Libp2p Kademlia DHT",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "start <protocol-ID>",
		Short: "Go onto the given Kademlia DHT. (connect to bootnode and refresh table)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(log) {
				return
			}
			if state.KadNode != nil {
				log.Error("Already have a Kademlia DHT open")
				return
			}
			ctx, cancel := context.WithCancel(r.Ctx)
			var err error
			state.KadNode, err = kad.NewKademlia(ctx, r, protocol.ID(args[0]))
			if err != nil {
				log.Error(err)
				return
			}
			state.CloseKad = cancel
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop kademlia",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if noKad(cmd) {
				return
			}
			state.CloseKad()
			state.KadNode = nil
			log.Info("Stopped kademlia DHT")
		},
	})

	refreshCmd := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh the Kademlia table. Optionally wait for it to complete.",
		Run: func(cmd *cobra.Command, args []string) {
			if noKad(cmd) {
				return
			}
			if err := state.KadNode.RefreshTable();  err != nil {
				log.Errorf("failed to refresh kad dht table: %v", err)
			} else {
				log.Info("successfully refreshed kad dht table")
			}
		},
	}
	cmd.AddCommand(refreshCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "find-conn-peers <peer-ID>",
		Short: "Find connected peer addresses of the given peer in the DHT",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noKad(cmd) {
				return
			}
			peerID, err := peer.Decode(args[0])
			if err != nil {
				log.Error(err)
				return
			}
			ctx, _ := context.WithTimeout(r.Ctx, time.Second*10)
			addrInfos, err := state.KadNode.FindPeersConnectedToPeer(ctx, peerID)
			if err != nil {
				log.Error(err)
				return
			}
			for {
				select {
				case <-ctx.Done():
					log.Infof("Timed out connected addrs query for peer %s", peerID.Pretty())
					return
				case info, ok := <-addrInfos:
					log.Infof("Found connected address for peer %s: %s", peerID.Pretty(), info.String())
					if !ok {
						log.Infof("Stopped connected addrs query for peer %s", peerID.Pretty())
						return
					}
				}
			}
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "find-peer <peer-ID>",
		Short: "Find address info of the given peer in the DHT",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noKad(cmd) {
				return
			}
			peerID, err := peer.Decode(args[0])
			if err != nil {
				log.Error(err)
				return
			}
			ctx, _ := context.WithTimeout(r.Ctx, time.Second*10)
			addrInfo, err := state.KadNode.FindPeer(ctx, peerID)
			if err != nil {
				log.Error(err)
				return
			}
			log.Infof("Found address for peer %s: %s", peerID.Pretty(), addrInfo.String())
		},
	})

	return cmd
}
