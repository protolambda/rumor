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
			h, hasHost := r.Host(log)
			if !hasHost {
				return
			}
			if state.KadNode != nil {
				log.Error("Already have a Kademlia DHT open")
				return
			}
			ctx, cancel := context.WithCancel(r.ActorCtx)
			var err error
			state.KadNode, err = kad.NewKademlia(ctx, h, protocol.ID(args[0]))
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
			if err := state.KadNode.RefreshTable(); err != nil {
				log.Errorf("failed to refresh kad dht table: %v", err)
			} else {
				log.Info("successfully refreshed kad dht table")
			}
		},
	}
	cmd.AddCommand(refreshCmd)

	{
		var timeout uint64
		findConnPeersCmd := &cobra.Command{
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
				findConnPeerCtx := ctx
				if timeout != 0 {
					findConnPeerCtx, _ = context.WithTimeout(findConnPeerCtx, time.Millisecond*time.Duration(timeout))
				}
				addrInfos, err := state.KadNode.FindPeersConnectedToPeer(findConnPeerCtx, peerID)
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
		}
		findConnPeersCmd.Flags().Uint64Var(&timeout, "timeout", 10_000, "Timeout to find anything, in milliseconds. Or 0 to disable.")
		cmd.AddCommand(findConnPeersCmd)
	}

	{
		var timeout uint64
		findPeerCmd := &cobra.Command{
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
				findPeerCtx := ctx
				if timeout != 0 {
					findPeerCtx, _ = context.WithTimeout(findPeerCtx, time.Millisecond*time.Duration(timeout))
				}
				addrInfo, err := state.KadNode.FindPeer(findPeerCtx, peerID)
				if err != nil {
					log.Error(err)
					return
				}
				log.WithField("addrs", addrInfo.Addrs).WithField("peer", peerID.Pretty()).Infof("Found addresses for peer")
			},
		}
		findPeerCmd.Flags().Uint64Var(&timeout, "timeout", 10_000, "Timeout to find a peer, in milliseconds. Or 0 to disable.")
		cmd.AddCommand(findPeerCmd)
	}

	return cmd
}
