package actor

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"strconv"
	"strings"
	"sync"
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
						v["latency"] = store.LatencyEWMA(p).Seconds() // A float, ok for json
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

func parseRoot(v string) ([32]byte, error) {
	if v == "0" {
		return [32]byte{}, nil
	}
	if strings.HasPrefix(v, "0x") {
		v = v[2:]
	}
	if len(v) != 64 {
		return [32]byte{}, fmt.Errorf("provided root has length %d, expected 64 hex characters (ignoring optional 0x prefix)", len(v))
	}
	var out [32]byte
	_, err := hex.Decode(out[:], []byte(v))
	return out, err
}

func parseForkVersion(v string) ([4]byte, error) {
	if strings.HasPrefix(v, "0x") {
		v = v[2:]
	}
	if len(v) != 8 {
		return [4]byte{}, fmt.Errorf("provided fork version has length %d, expected 8 hex characters (ignoring optional 0x prefix)", len(v))
	}
	var out [4]byte
	_, err := hex.Decode(out[:], []byte(v))
	return out, err
}

type PeerStatusState struct {
	Following bool
	Local     methods.Status
}

func (r *Actor) InitPeerStatusCmd(ctx context.Context, log logrus.FieldLogger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Manage and track peer status",
	}
	{
		var timeout uint64
		reqStatus := func(peerID peer.ID) error {
			h, hasHost := r.Host(log)
			if !hasHost {
				return nil
			}
			sFn := reqresp.NewStreamFn(h.NewStream)
			comp, err := readOptionalComp(cmd)
			if err != nil {
				return err
			}
			reqCtx := ctx
			if timeout != 0 {
				reqCtx, _ = context.WithTimeout(reqCtx, time.Millisecond*time.Duration(timeout))
			}
			m := methods.StatusRPCv1

			var reqStatus methods.Status
			if r.PeerStatusState.Following {
				// TODO get status from chain
			} else {
				reqStatus = r.PeerStatusState.Local
			}
			return m.RunRequest(reqCtx, sFn, peerID, comp,
				reqresp.RequestSSZInput{Obj: &reqStatus}, 1,
				func(chunk reqresp.ChunkedResponseHandler) error {
					resultCode := chunk.ResultCode()
					f := map[string]interface{}{
						"from":        peerID.String(),
						"result_code": resultCode,
					}
					switch resultCode {
					case reqresp.ServerErrCode, reqresp.InvalidReqCode:
						msg, err := chunk.ReadErrMsg()
						if err != nil {
							return err
						}
						f["msg"] = msg
					case reqresp.SuccessCode:
						var data methods.Status
						if err := chunk.ReadObj(&data); err != nil {
							return err
						}
						f["data"] = data
						inf, _ := r.GlobalPeerInfos.Find(peerID)
						inf.RegisterStatus(data)
					}
					log.WithFields(f).Debug("got status response")
					return nil
				})
		}
		{
			fetchCmd := &cobra.Command{
				Use:   "fetch <peerID>",
				Short: "Fetch status of connected peer.",
				Args:  cobra.ExactArgs(1),
				Run: func(cmd *cobra.Command, args []string) {
					peerID, err := peer.Decode(args[0])
					if err != nil {
						log.Error(err)
						return
					}
					if err := reqStatus(peerID); err != nil {
						log.Error(err)
						return
					}
				},
			}
			fetchCmd.Flags().String("compression", "none", "Optional compression. Try 'snappy' for streaming-snappy")
			fetchCmd.Flags().Uint64Var(&timeout, "timeout", 10_000, "Apply timeout of n milliseconds to each stream (complete request <> response time). 0 to Disable timeout.")

			cmd.AddCommand(fetchCmd)
		}
		{
			pollCmd := &cobra.Command{
				Use:   "poll <ms>",
				Short: "Fetch status of all connected peers, on interval of given milliseconds.",
				Args:  cobra.ExactArgs(1),
				Run: func(cmd *cobra.Command, args []string) {
					intervalMs, err := strconv.ParseUint(args[0], 0, 64)
					if err != nil {
						log.Errorf("could not parse interval: %v", err)
					}
					interval := time.Millisecond * time.Duration(intervalMs)
					h, hasHost := r.Host(log)
					if !hasHost {
						return
					}
					for {
						start := time.Now()
						var wg sync.WaitGroup
						for _, p := range h.Network().Peers() {
							// TODO: maybe filter peers that cannot answer status requests?
							wg.Add(1)
							go func(peerID peer.ID) {
								if err := reqStatus(peerID); err != nil {
									log.Warn(err)
								}
								wg.Done()
							}(p)
						}
						wg.Wait()
						pollStepDuration := time.Since(start)
						if pollStepDuration < interval {
							time.Sleep(interval - pollStepDuration)
						}
						select {
						case <-ctx.Done():
							return
						default:
							// next interval
						}
					}
				},
			}
			pollCmd.Flags().String("compression", "none", "Optional compression. Try 'snappy' for streaming-snappy")
			pollCmd.Flags().Uint64Var(&timeout, "timeout", 3_000, "Apply timeout of n milliseconds to each request. 0 to Disable timeout.")

			cmd.AddCommand(pollCmd)
		}
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "get",
		Short: "Get current status and if following the chain or not",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			log.WithFields(logrus.Fields{
				"following": r.PeerStatusState.Following,
				"status":    r.PeerStatusState.Local,
			}).Info("Status settings")
		},
	})

	{
		var forkVersion, headRoot, finalizedRoot string
		var headSlot, finalizedEpoch uint64
		var following bool
		setCmd := &cobra.Command{
			Use:   "set",
			Short: "Set (a part of) the current status and if following the chain or not",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				if cmd.Flags().Changed("fork-version") {
					v, err := parseForkVersion(forkVersion)
					if err != nil {
						log.Error(err)
					}
					r.PeerStatusState.Local.HeadForkVersion = v
				}
				if cmd.Flags().Changed("head-root") {
					v, err := parseRoot(headRoot)
					if err != nil {
						log.Error(err)
					}
					r.PeerStatusState.Local.HeadRoot = v
				}
				if cmd.Flags().Changed("head-slot") {
					r.PeerStatusState.Local.HeadSlot = methods.Slot(headSlot)
				}
				if cmd.Flags().Changed("finalized-epoch") {
					r.PeerStatusState.Local.FinalizedEpoch = methods.Epoch(finalizedEpoch)
				}
				if cmd.Flags().Changed("finalized-root") {
					v, err := parseRoot(finalizedRoot)
					if err != nil {
						log.Error(err)
					}
					r.PeerStatusState.Local.FinalizedRoot = v
				}
				if cmd.Flags().Changed("following") {
					r.PeerStatusState.Following = following
				}
				log.WithFields(logrus.Fields{
					"following": r.PeerStatusState.Following,
					"status":    r.PeerStatusState.Local,
				}).Info("Status settings")
			},
		}
		setCmd.Flags().StringVar(&forkVersion, "fork-version", "", "Fork version, hex encoded")
		setCmd.Flags().StringVar(&headRoot, "head-root", "", "Head root, hex encoded")
		setCmd.Flags().Uint64Var(&headSlot, "head-slot", 0, "Head slot")
		setCmd.Flags().StringVar(&finalizedRoot, "finalized-root", "", "Finalized root, hex encoded")
		setCmd.Flags().Uint64Var(&finalizedEpoch, "finalized-epoch", 0, "Finalized epoch")
		setCmd.Flags().BoolVar(&following, "following", false, "If the status should automatically follow the current chain (if any)")
	}

	{
		var timeout uint64
		serveCmd := &cobra.Command{
			Use:   "serve",
			Short: "Serve incoming status requests",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				h, hasHost := r.Host(log)
				if !hasHost {
					return
				}
				sCtxFn := func() context.Context {
					if timeout == 0 {
						return ctx
					}
					reqCtx, _ := context.WithTimeout(ctx, time.Millisecond*time.Duration(timeout))
					return reqCtx
				}
				comp, err := readOptionalComp(cmd)
				if err != nil {
					log.Error(err)
					return
				}
				listenReq := func(ctx context.Context, peerId peer.ID, handler reqresp.ChunkedRequestHandler) {
					f := map[string]interface{}{
						"from": peerId.String(),
					}
					var reqStatus methods.Status
					err := handler.ReadRequest(&reqStatus)
					if err != nil {
						f["input_err"] = err.Error()
						_ = handler.WriteInvalidRequestChunk("could not parse status request")
						log.WithFields(f).Warnf("failed to read status request: %v", err)
					} else {
						f["data"] = reqStatus
						inf, _ := r.GlobalPeerInfos.Find(peerId)
						inf.RegisterStatus(reqStatus)

						var resp methods.Status
						if r.PeerStatusState.Following {
							// TODO
						} else {
							resp = r.PeerStatusState.Local
						}
						if err := handler.WriteResponseChunk(&resp); err != nil {
							log.WithFields(f).Warnf("failed to respond to status request: %v", err)
						} else {
							log.WithFields(f).Warnf("handled status request: %v", err)
						}
					}
				}
				m := methods.StatusRPCv1
				streamHandler := m.MakeStreamHandler(sCtxFn, comp, listenReq)
				prot := m.Protocol
				if comp != nil {
					prot += protocol.ID("_" + comp.Name())
				}
				h.SetStreamHandler(prot, streamHandler)
				log.WithField("started", true).Infof("Opened listener")
				<-ctx.Done()
			},
		}
		serveCmd.Flags().String("compression", "none", "Optional compression. Try 'snappy' for streaming-snappy")
		serveCmd.Flags().Uint64Var(&timeout, "timeout", 10_000, "Apply timeout of n milliseconds to each stream (complete request <> response time). 0 to Disable timeout.")
		cmd.AddCommand(serveCmd)
	}
	return cmd
}

type PeerMetadataState struct {
	Following bool
	Local     methods.MetaData
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

		  poll <interval>  # poll connected peers for metadata by pinging them on interval

		  get
		  set --follow

		  serve   # serve meta data
	actors
	*/
	return cmd
}
