package actor

import (
	"context"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"time"
)

func (r *Actor) InitPeerCmd(ctx context.Context, log logrus.FieldLogger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "peer",
		Short: "Manage Libp2p peerstore",
	}
	{
		var listLatency, listProtocols, listAddrs, listStatus, listMetadata, listClaimSeq bool
		listCmd := &cobra.Command{
			Use:   "list [all|connected]",
			Short: "List peers in peerstore. Defaults to connected only.",
			Args:  cobra.ArbitraryArgs,
			Run: func(cmd *cobra.Command, args []string) {
				h, hasHost := r.Host(log)
				if !hasHost {
					return
				}
				if len(args) == 0 {
					args = append(args, "connected")
				}
				var peers []peer.ID
				switch args[0] {
				case "all":
					peers = h.Peerstore().Peers()
				case "connected":
					peers = h.Network().Peers()
				default:
					log.Errorf("invalid peer type: %s", args[0])
				}
				store := h.Peerstore()
				peerData := make(map[peer.ID]map[string]interface{})
				for _, p := range peers {
					v := make(map[string]interface{})
					if listAddrs {
						 v["addrs"] = store.PeerInfo(p).Addrs
					}
					// TODO: add dv5 node ID
					if listLatency {
						v["latency"] = store.LatencyEWMA(p).Seconds()  // A float, ok for json
					}
					if listProtocols {
						protocols, err := store.GetProtocols(p)
						if err != nil {
							v["protocols"] = protocols
						}
					}
					if listStatus || listMetadata || listClaimSeq {
						pInfoData, ok := r.GlobalPeerInfos.Find(p)
						if ok {
							if listStatus {
								v["status"] = pInfoData.Status()
							}
							if listMetadata {
								v["metadata"] = pInfoData.Metadata()
							}
							if listClaimSeq {
								v["metadata"] = pInfoData.ClaimedSeq()
							}
						}
					}
					peerData[p] = v
				}
				log.WithField("peers", peerData).Infof("%d peers", len(peers))
			},
		}
		flags := listCmd.Flags()
		flags.BoolVar(&listLatency, "latency", false, "list peer latency")
		flags.BoolVar(&listProtocols, "protocols", false, "list peer protocols")
		flags.BoolVar(&listAddrs, "addrs", true, "list peer addrs")
		flags.BoolVar(&listStatus, "status", false, "list peer status")
		flags.BoolVar(&listMetadata, "metadata", false, "list peer metadata")
		flags.BoolVar(&listClaimSeq, "claimseq", false, "list peer claimed metadata seq nr")
		cmd.AddCommand(listCmd)
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "trim",
		Short: "Trim peers (2 second time allowance)",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			h, hasHost := r.Host(log)
			if !hasHost {
				return
			}
			ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
			h.ConnManager().TrimOpenConns(ctx)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "connect <addr> [<tag>]",
		Short: "Connect to peer. Addr can be a multi-addr, enode or ENR",
		Args:  cobra.RangeArgs(1, 2),
		Run: func(cmd *cobra.Command, args []string) {
			h, hasHost := r.Host(log)
			if !hasHost {
				return
			}
			addrStr := args[0]
			var muAddr ma.Multiaddr
			if dv5Addr, err := addrutil.ParseEnrOrEnode(addrStr); err != nil {
				muAddr, err = ma.NewMultiaddr(args[0])
				if err != nil {
					log.Info("addr not an enode or multi addr")
					log.Error(err)
					return
				}
			} else {
				muAddr, err = addrutil.EnodeToMultiAddr(dv5Addr)
				if err != nil {
					log.Error(err)
					return
				}
			}
			addrInfo, err := peer.AddrInfoFromP2pAddr(muAddr)
			if err != nil {
				log.Error(err)
				return
			}
			ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
			if err := h.Connect(ctx, *addrInfo); err != nil {
				log.Error(err)
				return
			}
			log.WithField("peer_id", addrInfo.ID.Pretty()).Infof("connected to peer")
			if len(args) > 1 {
				h.ConnManager().Protect(addrInfo.ID, args[1])
				log.Infof("protected peer %s as tag %s", addrInfo.ID.Pretty(), args[1])
			}
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "disconnect <peerID>",
		Short: "Disconnect peer",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			h, hasHost := r.Host(log)
			if !hasHost {
				return
			}
			peerID, err := peer.Decode(args[0])
			if err != nil {
				log.Error(err)
				return
			}
			conns := h.Network().ConnsToPeer(peerID)
			for _, c := range conns {
				if err := c.Close(); err != nil {
					log.Infof("error during disconnect of peer %s (%s)", peerID.Pretty(), c.RemoteMultiaddr().String())
				}
			}
			log.Infof("disconnected peer %s", peerID.Pretty())
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "protect <peerID> <tag>",
		Short: "Protect peer, tagging them as <tag>",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			h, hasHost := r.Host(log)
			if !hasHost {
				return
			}
			peerID, err := peer.Decode(args[0])
			if err != nil {
				log.Error(err)
				return
			}
			tag := args[1]
			h.ConnManager().Protect(peerID, tag)
			log.Infof("protected peer %s as %s", peerID.Pretty(), tag)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "unprotect <peerID> <tag>",
		Short: "Unprotect peer, un-tagging them as <tag>",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			h, hasHost := r.Host(log)
			if !hasHost {
				return
			}
			peerID, err := peer.Decode(args[0])
			if err != nil {
				log.Error(err)
				return
			}
			tag := args[1]
			h.ConnManager().Unprotect(peerID, tag)
			log.Infof("un-protected peer %s as %s", peerID.Pretty(), tag)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "addrs [peerID]",
		Short: "View known addresses of [peerID]. Defaults to local addresses if no peer id is specified.",
		Args:  cobra.RangeArgs(0, 1),
		Run: func(cmd *cobra.Command, args []string) {
			h, hasHost := r.Host(log)
			if !hasHost {
				return
			}
			if len(args) > 0 {
				peerID, err := peer.Decode(args[0])
				if err != nil {
					log.Error(err)
					return
				}
				addrs := h.Peerstore().Addrs(peerID)
				log.WithField("addrs", addrs).Infof("addrs for peer %s", peerID.Pretty())
			} else {
				addrs := h.Addrs()
				log.WithField("addrs", addrs).Infof("host addrs")
			}
		},
	})
	return cmd
}

type PeerStatusState struct {
	Following bool
	Local methods.Status
}

func (r *Actor) InitPeerStatusCmd(ctx context.Context, log logrus.FieldLogger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Manage and track peer status",
	}
	/* TODO
				fetch <peer id>

				poll <interval>  # poll connected peers for status by sending them repeated status requests

				get
				set
				follow

				serve   # handle status requests on RPC and respond based on chain
	*/
	cmd.AddCommand(&cobra.Command{
		Use:   "fetch <peerID>",
		Short: "Fetch status of connected peer.",
		Args:  cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			// TODO do status request, using current status (fetch from current chain if following)
			// TODO register status in global peer infos
			// TODO log resulting status
		},
	})

	return cmd
}

type PeerMetadataState struct {
	Following bool
	Local methods.MetaData
}

func (r *Actor) InitPeerMetadataCmd(ctx context.Context, log logrus.FieldLogger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metadata",
		Short: "Manage and track peer metadata",
	}
	/* TODO
		  ping <peer id>  --update # request peer for pong, update metadata maybe
		  pong --update   # serve others with pongs, and if ping is new enough, request them for metadata if --update=true

		  fetch <peer id>  # get metadata of peer
		  list

		  poll <interval>  # poll connected peers for metadata by pinging them on interval

		  get
		  set
		  follow

		  serve   # serve meta data
	actors
	*/
	return cmd
}
