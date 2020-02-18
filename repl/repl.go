package repl

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/protolambda/rumor/addrutil"
	"github.com/protolambda/rumor/gossip"
	"github.com/protolambda/rumor/node"
	"github.com/protolambda/rumor/peering/dv5"
	"github.com/protolambda/rumor/peering/kad"
	"github.com/protolambda/rumor/peering/static"
	"fmt"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	mplex "github.com/libp2p/go-libp2p-mplex"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	secio "github.com/libp2p/go-libp2p-secio"
	yamux "github.com/libp2p/go-libp2p-yamux"
	"github.com/libp2p/go-tcp-transport"
	ws "github.com/libp2p/go-ws-transport"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/protolambda/rumor/rpc/methods"
	"github.com/protolambda/rumor/rpc/reqresp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

type Repl struct {
	P2PHost host.Host
	PrivKey crypto.PrivKey
	Log     logrus.FieldLogger
	Ctx     context.Context
	Cancel  context.CancelFunc
	ReplCmd *cobra.Command
}

// check interface
var _ = (node.Node)((*Repl)(nil))

func NewRepl(log logrus.FieldLogger) *Repl {
	repl := &Repl{
		Log: log,
	}
	{
		ctxAll, cancelAll := context.WithCancel(context.Background())
		repl.Ctx = ctxAll
		repl.Cancel = cancelAll
	}
	cmd := &cobra.Command{
		Use:   "",
		Short: "A REPL for Eth2 networking.",
		Long:  `A REPL for Eth2 networking. For debugging and interacting with Eth2 network components.`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	cmd.AddCommand(
		repl.InitHostCmd(),
		repl.InitEnrCmd(),
		repl.InitPeerCmd(),
		repl.InitDv5Cmd(),
		repl.InitKadCmd(),
		repl.InitGossipCmd(),
		repl.InitRpcCmd(),
	)
	repl.ReplCmd = cmd
	return repl
}

func (r *Repl) Host() host.Host {
	return r.P2PHost
}

func (r *Repl) Logger(logTopic string) logrus.FieldLogger {
	return r.Log.WithField("log_topic", logTopic)
}

func writeErrMsg(cmd *cobra.Command, format string, a ...interface{}) {
	_, _ = fmt.Fprintf(cmd.OutOrStderr(), format+"\n", a...)
}

func writeErr(cmd *cobra.Command, err error) {
	_, _ = fmt.Fprintf(cmd.OutOrStderr(), err.Error()+"\n")
}

func (r *Repl) NoHost(cmd *cobra.Command) bool {
	if r.P2PHost == nil {
		writeErrMsg(cmd, "REPL must have initialized Libp2p host. Try 'host start'")
		return true
	}
	return false
}

func (r *Repl) InitHostCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "host",
		Short: "Manage host",
	}
	var privKeyStr string
	var transportsStrArr []string
	var muxStrArr []string
	var securityStr string
	var relayEnabled bool
	var loPeers, hiPeers int
	var gracePeriodMs int

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the host node. See flags for security, transport, mux etc. options",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if r.P2PHost != nil {
				writeErrMsg(cmd, "Already have a host open.")
				return
			}
			{
				if privKeyStr == "" { // generate new private key if non was specified
					var err error
					r.PrivKey, _, err = crypto.GenerateKeyPairWithReader(crypto.Secp256k1, -1, rand.Reader)
					if err != nil {
						writeErr(cmd, err)
						return
					}
					p, err := crypto.MarshalPrivateKey(r.PrivKey)
					if err != nil {
						writeErr(cmd, err)
						return
					}
					r.Logger("host-key").Infof("Generated random Secp256k1 private key: %s", hex.EncodeToString(p))
				} else {
					privKeyBytes, err := hex.DecodeString(privKeyStr)
					if err != nil {
						writeErrMsg(cmd, "cannot parse private key, expected hex string (without 0x)")
						return
					}
					r.PrivKey, err = crypto.UnmarshalSecp256k1PrivateKey(privKeyBytes)
					if err != nil {
						writeErrMsg(cmd, "cannot parse private key, invalid private key (Secp256k1)")
						return
					}
				}
			}
			hostOptions := make([]libp2p.Option, 0)

			for _, v := range transportsStrArr {
				v = strings.ToLower(strings.TrimSpace(v))
				switch v {
				case "tcp":
					hostOptions = append(hostOptions, libp2p.Transport(tcp.NewTCPTransport))
				case "ws":
					hostOptions = append(hostOptions, libp2p.Transport(ws.New))
				default:
					writeErrMsg(cmd, "could not recognize transport %s", v)
					return
				}
			}

			for _, v := range muxStrArr {
				v = strings.ToLower(strings.TrimSpace(v))
				switch v {
				case "yamux":
					hostOptions = append(hostOptions, libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport))
				case "mplex":
					hostOptions = append(hostOptions, libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport))
				default:
					writeErrMsg(cmd, "could not recognize mux %s", v)
					return
				}
			}

			{
				switch securityStr {
				case "none":
					// no security, for debugging etc.
				case "secio":
					hostOptions = append(hostOptions, libp2p.Security(secio.ID, secio.New))
				default:
					writeErrMsg(cmd, "could not recognize security %s", securityStr)
					return
				}
			}

			if relayEnabled {
				hostOptions = append(hostOptions, libp2p.EnableRelay())
			}
			hostOptions = append(hostOptions,
				libp2p.Identity(r.PrivKey),
				libp2p.Peerstore(pstoremem.NewPeerstore()), // TODO: persist peerstore?
				libp2p.ConnectionManager(connmgr.NewConnManager(loPeers, hiPeers, time.Millisecond*time.Duration(gracePeriodMs))),
			)
			h, err := libp2p.New(r.Ctx, hostOptions...)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			r.P2PHost = h
		},
	}
	startCmd.Flags().StringVar(&privKeyStr, "priv-key", "", "hex-encoded RSA private key for libp2p host. Random if none is specified.")
	startCmd.Flags().StringArrayVar(&muxStrArr, "mux", []string{"yamux", "mplex"}, "Multiplexers to use")
	startCmd.Flags().StringArrayVar(&transportsStrArr, "transports", []string{"tcp"}, "Transports to use. Options: tcp, ws")
	startCmd.Flags().StringVar(&securityStr, "security", "secio", "Security to use. Options: secio, none")
	startCmd.Flags().BoolVar(&relayEnabled, "relay", false, "enable relayer functionality")
	startCmd.Flags().IntVar(&loPeers, "lo-peers", 15, "low-water for connection manager to trim peer count to")
	startCmd.Flags().IntVar(&hiPeers, "hi-peers", 20, "high-water for connection manager to trim peer count from")
	startCmd.Flags().IntVar(&gracePeriodMs, "peer-grace-period", 20_000, "Time in milliseconds to grace a peer from being trimmed")

	cmd.AddCommand(startCmd)
	cmd.AddCommand(&cobra.Command{
		Use:   "listen <multi-addr> [multi-addr [multi-addr [...]]]",
		Short: "Start listening on given addresses",
		Args:  cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(cmd) {
				return
			}
			if len(args) == 0 {
				args = append(args, "/ip4/0.0.0.0/tcp/0", "/ip6/::/tcp/0")
			}
			maddrs, err := static.ParseMultiAddrs(args...)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			if err := r.Host().Network().Listen(maddrs...); err != nil {
				writeErr(cmd, err)
				return
			}
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "view",
		Short: "View local peer ID, listening addresses, etc.",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(cmd) {
				return
			}
			log := r.Logger("host")
			h := r.Host()
			log.Infof("Peer ID: %s", h.ID().Pretty())
			for _, a := range h.Addrs() {
				log.Infof("Listening on: %s", a.String())
			}
			log.Infof("Security: %s,  Mux: %s,  Transports: %s,  Relay: %v",
				strings.ToLower(securityStr),
				strings.ToLower(strings.Join(muxStrArr, ", ")),
				strings.ToLower(strings.Join(transportsStrArr, ", ")),
				relayEnabled)
		},
	})
	return cmd
}

func (r *Repl) InitEnrCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enr",
		Short: "Ethereum Name Record (ENR) utilities",
	}
	log := r.Logger("enr")
	cmd.AddCommand(&cobra.Command{
		Use:   "view <enr>",
		Short: "view ENR contents. ENR is url-base64 (RFC 4648). With optional 'enr:' or 'enr://' prefix.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			enrStr := args[0]
			rec, err := addrutil.ParseEnr(enrStr)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			enrPairs, err := addrutil.ParseEnrInternals(rec)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			for _, p := range enrPairs {
				ent, err := p.ValueEntry()
				if err != nil {
					log.Infof("Enr pair: %s -> (could not decode) -- raw: %x", p.K, p.V)
				} else {
					log.Infof("Enr pair: %s -> %s -- raw: %x", p.K, ent, p.V)
				}
			}
		},
	})
	return cmd
}

func (r *Repl) InitPeerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "peer",
		Short: "Manage Libp2p peerstore",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list <all,connected>",
		Short: "List peers in peerstore. Defaults to connected only.",
		Args:  cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(cmd) {
				return
			}
			if len(args) == 0 {
				args = append(args, "connected")
			}
			var peers []peer.ID
			switch args[0] {
			case "all":
				peers = r.P2PHost.Peerstore().Peers()
			case "connected":
				peers = r.P2PHost.Network().Peers()
			default:
				writeErrMsg(cmd, "invalid peer type: %s", args[0])
			}
			log := r.Logger("peers")
			log.Infof("%d peers", len(peers))
			for i, p := range peers {
				log.Infof("%4d: %s", i, r.P2PHost.Peerstore().PeerInfo(p).String())
			}
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "trim",
		Short: "Trim peers (2 second time allowance)",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(cmd) {
				return
			}
			ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
			r.P2PHost.ConnManager().TrimOpenConns(ctx)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "connect <addr> [<tag>]",
		Short: "Connect to peer. Addr can be a multi-addr, enode or ENR",
		Args:  cobra.RangeArgs(1, 2),
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(cmd) {
				return
			}
			log := r.Logger("connect-peer")
			addrStr := args[0]
			var muAddr ma.Multiaddr
			if dv5Addr, err := parseDv5Addr(addrStr); err != nil {
				log.Info("addr not a dv5 addr")
				muAddr, err = ma.NewMultiaddr(args[0])
				if err != nil {
					log.Info("addr not a multi addr either")
					writeErr(cmd, err)
					return
				}
			} else {
				log.Info("addr is a dv5 addr")
				muAddr, err = dv5.Dv5NodeToMultiAddr(dv5Addr)
				if err != nil {
					writeErr(cmd, err)
					return
				}
			}
			log.Infof("parsed multi addr: %s", muAddr.String())
			addrInfo, err := peer.AddrInfoFromP2pAddr(muAddr)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
			if err := r.P2PHost.Connect(ctx, *addrInfo); err != nil {
				writeErr(cmd, err)
				return
			}
			log.Infof("connected to peer %s", addrInfo.ID.Pretty())
			if len(args) > 1 {
				r.P2PHost.ConnManager().Protect(addrInfo.ID, args[1])
				log.Infof("protected peer %s as tag %s", addrInfo.ID.Pretty(), args[1])
			}
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "disconnect <peerID>",
		Short: "Disconnect peer",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(cmd) {
				return
			}
			peerID, err := peer.Decode(args[0])
			if err != nil {
				writeErr(cmd, err)
				return
			}
			conns := r.P2PHost.Network().ConnsToPeer(peerID)
			log := r.Logger("disconnect-peer")
			for _, c := range conns {
				if err := c.Close(); err != nil {
					log.Infof("error during disconnect of peer %s (%s)", peerID.Pretty(), c.RemoteMultiaddr().String())
				}
			}
			log.Infof("finished disconnecting peer %s", peerID.Pretty())
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "protect <peerID> <tag>",
		Short: "Protect peer, tagging them as <tag>",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(cmd) {
				return
			}
			peerID, err := peer.Decode(args[0])
			if err != nil {
				writeErr(cmd, err)
				return
			}
			log := r.Logger("protect-peer")
			tag := args[1]
			r.P2PHost.ConnManager().Protect(peerID, tag)
			log.Infof("protected peer %s as %s", peerID.Pretty(), tag)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "unprotect <peerID> <tag>",
		Short: "Unprotect peer, un-tagging them as <tag>",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(cmd) {
				return
			}
			peerID, err := peer.Decode(args[0])
			if err != nil {
				writeErr(cmd, err)
				return
			}
			log := r.Logger("protect-peer")
			tag := args[1]
			r.P2PHost.ConnManager().Unprotect(peerID, tag)
			log.Infof("protected peer %s as %s", peerID.Pretty(), tag)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "addrs [peerID]",
		Short: "View known addresses of [peerID]. Defaults to local addresses if no peer id is specified.",
		Args:  cobra.RangeArgs(0, 1),
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(cmd) {
				return
			}
			log := r.Logger("peer-addrs")
			if len(args) > 0 {
				peerID, err := peer.Decode(args[0])
				if err != nil {
					writeErr(cmd, err)
					return
				}
				addrs := r.P2PHost.Peerstore().Addrs(peerID)
				for i, a := range addrs {
					log.Infof("%s addr #%d: %s", peerID.Pretty(), i, a.String())
				}
				if len(addrs) == 0 {
					log.Infof("no known addrs for peer %s", peerID.Pretty())
				}
			} else {
				addrs := r.P2PHost.Addrs()
				for i, a := range addrs {
					log.Infof("host addr #%d: %s", i, a.String())
				}
				if len(addrs) == 0 {
					log.Info("no host addrs")
				}
			}
		},
	})
	return cmd
}

func parseDv5Addr(v string) (*discv5.Node, error) {
	if strings.HasPrefix(v, "enode://") {
		addr := new(discv5.Node)
		err := addr.UnmarshalText([]byte(v))
		if err != nil {
			return nil, err
		}
		return addr, nil
	} else {
		enrAddr, err := addrutil.ParseEnr(v)
		if err != nil {
			return nil, err
		}
		// TODO: warn if no "eth2" key in ENR record.
		enodeAddr, err := addrutil.EnrToEnode(enrAddr, true)
		if err != nil {
			return nil, err
		}
		return dv5.EnodeToDiscv5Node(enodeAddr)
	}
}

func (r *Repl) InitDv5Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dv5",
		Short: "Manage Ethereum Discv5",
	}
	log := r.Logger("discv5")

	var dv5Node dv5.Discv5
	var closeDv5 context.CancelFunc

	noDv5 := func(cmd *cobra.Command) bool {
		if r.NoHost(cmd) {
			return true
		}
		if dv5Node == nil {
			writeErrMsg(cmd, "REPL must have initialized discv5. Try 'dv5 start'")
			return true
		}
		return false
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "start <UDP-address-with-port>",
		Short: "Start discv5.",
		Long:  "Start discv5.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(cmd) {
				return
			}
			if dv5Node != nil {
				writeErrMsg(cmd, "Already have dv5 open at %s", dv5Node.UDPAddress().String())
				return
			}
			ctx, cancel := context.WithCancel(r.Ctx)
			var err error
			dv5Node, err = dv5.NewDiscV5(ctx, r, args[0], r.PrivKey)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			closeDv5 = cancel
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop discv5",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			closeDv5()
			dv5Node = nil
			log.Info("Stopped discv5")
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "bootstrap <addr>",
		Short: "Bootstrap discv5 by connecting to the given discv5 node.",
		Long:  "Dv5 addr can be an enode or ENR. Enode format example: 'enode://<hex node id>@10.3.58.6:30303?discport=30301'. enr is url-base64 encoded, optionally prefixed with 'enr:'.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			dv5Addr, err := parseDv5Addr(args[0])
			if err != nil {
				writeErr(cmd, err)
				return
			}
			if err := dv5Node.AddDiscV5BootNodes([]*discv5.Node{dv5Addr}); err != nil {
				writeErr(cmd, err)
				return
			}
			log.Infof("bootstrapped to %s", dv5Addr.String())
		},
	})
	// TODO: Discv5 topic functionality is unstable and not specced yet.
	//cmd.AddCommand(&cobra.Command{
	//	Use:   "register <topic>",
	//	Short: "Register a topic",
	//	Run: func(cmd *cobra.Command, args []string) {
	//
	//	},
	//})
	//cmd.AddCommand(&cobra.Command{
	//	Use:   "start",
	//	Short: "Actively query for new discv5 nodes",
	//	Args: cobra.NoArgs,
	//	Run: func(cmd *cobra.Command, args []string) {
	//
	//	},
	//})
	connectNearby := false
	nearbyCmd := &cobra.Command{
		Use:   "nearby [target node: raw node ID (hex) or enode address or ENR (url-base64)]",
		Short: "Get list of nearby multi addrs (excluding self). If no target node is provided, then find nodes nearby to self.",
		Args:  cobra.RangeArgs(0, 1),
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			target := dv5Node.Self()
			if len(args) > 0 {
				if n, err := parseDv5Addr(args[0]); err != nil {
					if h, err := hex.DecodeString(args[0]); err != nil {
						writeErrMsg(cmd, "provided target node is not a valid node ID, enode address or ENR")
						return
					} else {
						if len(h) != 64 {
							writeErrMsg(cmd, "hex node ID is not 64 bytes")
							return
						} else {
							copy(target[:], h)
						}
					}
				} else {
					target = n.ID
				}
			}
			var nearby []*discv5.Node
			// filter out self from results
			selfID := dv5Node.Self()
			for _, v := range dv5Node.NearNodes(target) {
				if v.ID != selfID {
					nearby = append(nearby, v)
				}
			}

			dv5AddrsOut := ""
			for i, v := range nearby {
				if i > 0 {
					dv5AddrsOut += "  "
				}
				dv5AddrsOut += v.String()
			}
			log := r.Logger("discv5")
			log.Infof("nearby nodes to target %s: %s", target.String(), dv5AddrsOut)
			mAddrs, err := dv5.Dv5NodesToMultiAddrs(nearby)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			mAddrsOut := ""
			for i, v := range mAddrs {
				if i > 0 {
					mAddrsOut += "  "
				}
				mAddrsOut += v.String()
			}
			log.Infof("addresses of nearby nodes (%d): %s", len(mAddrs), mAddrsOut)
			if connectNearby {
				log.Infof("connecting to nearby nodes (with a 10 second timeout)")
				ctx, _ := context.WithTimeout(r.Ctx, time.Second*10)
				err := static.ConnectStaticPeers(ctx, r, mAddrs, func(info peer.AddrInfo, alreadyConnected bool) error {
					log.Infof("connected to peer from discv5 nearby nodes: %s", info.String())
					return nil
				})
				if err != nil {
					writeErr(cmd, err)
				}
			}
		},
	}
	nearbyCmd.Flags().BoolVar(&connectNearby, "connect", false, "Connect to the discv5 nearby nodes")
	cmd.AddCommand(nearbyCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "self",
		Short: "get local discv5 nodeID and udp address",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			log.Infof("local dv5 node: enode://%s@%s", dv5Node.Self().String(), dv5Node.UDPAddress().String())
		},
	})
	return cmd
}

func (r *Repl) InitKadCmd() *cobra.Command {
	var kadNode kad.Kademlia
	var closeKad context.CancelFunc

	noKad := func(cmd *cobra.Command) bool {
		if r.NoHost(cmd) {
			return true
		}
		if kadNode == nil {
			writeErrMsg(cmd, "REPL must have initialized Kademlia DHT. Try 'kad start'")
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
			if r.NoHost(cmd) {
				return
			}
			if kadNode != nil {
				writeErrMsg(cmd, "Already have a Kademlia DHT open")
				return
			}
			ctx, cancel := context.WithCancel(r.Ctx)
			var err error
			kadNode, err = kad.NewKademlia(ctx, r, protocol.ID(args[0]))
			if err != nil {
				writeErr(cmd, err)
				return
			}
			closeKad = cancel
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
			closeKad()
			kadNode = nil
			r.Logger("kad").Info("Stopped kademlia DHT")
		},
	})

	var waitForRefreshResult bool
	refreshCmd := &cobra.Command{
		Use:   "refresh [--wait]",
		Short: "Refresh the Kademlia table. Optionally wait for it to complete.",
		Run: func(cmd *cobra.Command, args []string) {
			if noKad(cmd) {
				return
			}
			kadNode.RefreshTable(waitForRefreshResult)
		},
	}
	refreshCmd.Flags().BoolVar(&waitForRefreshResult, "wait", false, "Wait for the table refresh to complete")
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
				writeErr(cmd, err)
				return
			}
			ctx, _ := context.WithTimeout(r.Ctx, time.Second*10)
			addrInfos, err := kadNode.FindPeersConnectedToPeer(ctx, peerID)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			for {
				select {
				case <-ctx.Done():
					r.Logger("kad").Infof("Timed out connected addrs query for peer %s", peerID.Pretty())
					return
				case info, ok := <-addrInfos:
					r.Logger("kad").Infof("Found connected address for peer %s: %s", peerID.Pretty(), info.String())
					if !ok {
						r.Logger("kad").Infof("Stopped connected addrs query for peer %s", peerID.Pretty())
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
				writeErr(cmd, err)
				return
			}
			ctx, _ := context.WithTimeout(r.Ctx, time.Second*10)
			addrInfo, err := kadNode.FindPeer(ctx, peerID)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			r.Logger("kad").Infof("Found address for peer %s: %s", peerID.Pretty(), addrInfo.String())
		},
	})

	return cmd
}

type joinedTopics map[string]*pubsub.Topic

type topicLogger struct {
	name      string
	topicName string
	outPath   string
	close     context.CancelFunc
}

type topicLoggers map[string]*topicLogger

func (r *Repl) InitGossipCmd() *cobra.Command {
	var gsNode gossip.GossipSub
	var closeGS context.CancelFunc

	var topics joinedTopics

	log := r.Logger("gossip")

	noGS := func(cmd *cobra.Command) bool {
		if r.NoHost(cmd) {
			return true
		}
		if gsNode == nil {
			writeErrMsg(cmd, "REPL must have initialized GossipSub. Try 'gossip start'")
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
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(cmd) {
				return
			}
			if gsNode != nil {
				writeErrMsg(cmd, "Already started GossipSub")
				return
			}
			topics = make(joinedTopics)
			ctx, cancel := context.WithCancel(r.Ctx)
			var err error
			gsNode, err = gossip.NewGossipSub(ctx, r)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			closeGS = cancel
			log.Info("Started GossipSub")
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop GossipSub",
		Run: func(cmd *cobra.Command, args []string) {
			if noGS(cmd) {
				return
			}
			gsNode = nil

			closeGS()
			log.Info("Stopped GossipSub")
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List joined gossip topics",
		Run: func(cmd *cobra.Command, args []string) {
			if noGS(cmd) {
				return
			}
			log.Infof("on %d topics:", len(topics))
			for topic, _ := range topics {
				log.Infof("topic: %s", topic)
			}
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "join <topic>",
		Short: "Join a gossip topic. Propagate anything.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noGS(cmd) {
				return
			}
			topicName := args[0]
			if _, ok := topics[topicName]; ok {
				writeErrMsg(cmd, "already on gossip topic %s", topicName)
				return
			}
			top, err := gsNode.JoinTopic(topicName)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			evHandler, err := top.EventHandler()
			if err != nil {
				writeErr(cmd, err)
				// no events, but still joined, don't exit
			} else {
				go func() {
					log.Infof("Started listening for peer join/leave events for topic %s", topicName)
					for {
						ev, err := evHandler.NextPeerEvent(context.Background())
						if err != nil {
							log.Infof("Stopped listening for peer join/leave events for topic %s", topicName)
							return
						}
						switch ev.Type {
						case pubsub.PeerJoin:
							log.Infof("peer %s joined topic %s", ev.Peer.Pretty(), topicName)
						case pubsub.PeerLeave:
							log.Infof("peer %s left topic %s", ev.Peer.Pretty(), topicName)
						}
					}
				}()
			}

			topics[topicName] = top
			log.Infof("joined topic %s", topicName)
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
			if top, ok := topics[topicName]; !ok {
				writeErrMsg(cmd, "not on gossip topic %s", topicName)
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
				writeErr(cmd, err)
				return
			}
			gsNode.BlacklistPeer(peerID)
			log.Infof("Blacklisted peer %s", peerID.Pretty())
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "lurk <topic>",
		Short: "Lurk a gossip topic. Propagate nothing.",
		Run: func(cmd *cobra.Command, args []string) {
			// TODO
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
			if top, ok := topics[topicName]; !ok {
				writeErrMsg(cmd, "not on gossip topic %s", topicName)
				return
			} else {
				err := top.Close()
				if err != nil {
					writeErr(cmd, err)
				}
				delete(topics, topicName)
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
		return gsNode.LogTopic(ctx, loggerName, top, msgLogger, errLogger)
	}

	topLogs := make(topicLoggers)

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
			if top, ok := topics[topicName]; !ok {
				writeErrMsg(cmd, "not on gossip topic %s", topicName)
				return
			} else {
				ctx, cancelLogger := context.WithCancel(r.Ctx)
				if err := logToFile(ctx, loggerName, top, outPath); err != nil {
					writeErr(cmd, err)
					return
				}
				topLogs[loggerName] = &topicLogger{
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
			if lo, ok := topLogs[loggerName]; ok {
				lo.close()
				log.Infof("Closed logger '%s', topic: '%s', output path: '%s'", loggerName, lo.topicName, lo.outPath)
				delete(topLogs, loggerName)
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
			log.Infof("%d loggers", len(topLogs))
			for name, lo := range topLogs {
				log.Infof("%20s: '%s' -> '%s'", name, lo.topicName, lo.outPath)
			}
		},
	})
	cmd.AddCommand(logCmd)
	return cmd
}

func (r *Repl) InitRpcCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rpc",
		Short: "Manage Eth2 RPC",
	}
	log := r.Logger("rpc")

	makeReqCmd := func(cmd *cobra.Command,
		rpcMethod func(cmd *cobra.Command) *reqresp.RPCMethod,
		mkReq func(cmd *cobra.Command, args []string) (interface{}, error),
		onResp func(peerID peer.ID, chunkIndex uint64, responseCode uint8, readChunk func(dest interface{}) error) error,
		onClose func(peerID peer.ID),
	) *cobra.Command {
		cmd.Flags().String("compression", "none", "Optional compression. Try 'snappy' for streaming-snappy")
		cmd.Run = func(cmd *cobra.Command, args []string) {
			if r.NoHost(cmd) {
				return
			}
			sFn := func(ctx context.Context, peerId peer.ID, protocolId protocol.ID) (network.Stream, error) {
				return r.P2PHost.NewStream(ctx, peerId, protocolId)
			}
			ctx, _ := context.WithTimeout(r.Ctx, time.Second*10) // TODO add timeout option
			peerID, err := peer.Decode(args[0])
			if err != nil {
				writeErr(cmd, err)
				return
			}
			var comp reqresp.Compression = nil
			if compStr, err := cmd.Flags().GetString("compression"); err != nil {
				writeErr(cmd, err)
				return
			} else {
				switch compStr {
				case "none", "", "false":
					// no compression
				case "snappy":
					comp = reqresp.SnappyCompression{}
				default:
					writeErrMsg(cmd, "cannot recognize compression '%s'", compStr)
				}
			}
			req, err := mkReq(cmd, args)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			lastRespChunkIndex := int64(-1)
			if err := rpcMethod(cmd).RunRequest(ctx, sFn, peerID, comp, req,
				func(chunkIndex uint64, responseCode uint8, readChunk func(dest interface{}) error) error {
					log.Debugf("Received response chunk %d with code %d from peer %s", chunkIndex, responseCode, peerID.Pretty())
					lastRespChunkIndex = int64(chunkIndex)
					return onResp(peerID, chunkIndex, responseCode, readChunk)
				}, func() {
					log.Debugf("Responses of peer %s stopped after %d response chunks", peerID.Pretty(),lastRespChunkIndex+1)
					onClose(peerID)
				}); err != nil {
				writeErr(cmd, err)
			}
		}
		return cmd
	}
	cmd.AddCommand(makeReqCmd(&cobra.Command{
		Use:   "goodbye <peerID> <code>",
		Short: "Send a goodbye to a peer, optionally disconnecting the peer after sending the Goodbye.",
		Args: cobra.ExactArgs(2),
	}, func(cmd *cobra.Command) *reqresp.RPCMethod {
		return &methods.GoodbyeRPCv1
	}, func(cmd *cobra.Command, args []string) (interface{}, error) {
			v, err := strconv.ParseUint(args[1], 0, 64)
			if err != nil {
				return nil, fmt.Errorf("failed to parse Goodbye code '%s'", args[1])
			}
			req := methods.Goodbye(v)
			return &req, nil
		}, func(peerID peer.ID, chunkIndex uint64, responseCode uint8, readChunk func(dest interface{}) error) error {
			if chunkIndex > 0 {
				return fmt.Errorf("unexpected second Goodbye response chunk with code %d from peer %s", responseCode, peerID.Pretty())
			}
			var data methods.Goodbye
			if err := readChunk(&data); err != nil {
				return err
			}
			log.Infof("Goodbye RPC response of peer %s: %d", peerID.Pretty(), data)
			return nil
		}, func(peerID peer.ID) {
			log.Infof("Goodbye RPC responses of peer %s ended", peerID.Pretty())
		},
	))
	parseRoot := func(v string) ([32]byte, error) {
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
	parseForkVersion := func(v string) ([4]byte, error) {
		if strings.HasPrefix(v, "0x") {
			v = v[2:]
		}
		if len(v) != 8 {
			return [4]byte{}, fmt.Errorf("provided root has length %d, expected 8 hex characters (ignoring optional 0x prefix)", len(v))
		}
		var out [4]byte
		_, err := hex.Decode(out[:], []byte(v))
		return out, err
	}
	blocksByRangeCmd := makeReqCmd(&cobra.Command{
		Use:   "blocks-by-range <peerID> <start-slot> <count> <step> [head-root-hex]",
		Short: "Get blocks by range from a peer. The head-root is optional, and defaults to zeroes. Use --v2 for no head-root.",
		Args: cobra.RangeArgs(4, 5),
	}, func(cmd *cobra.Command) *reqresp.RPCMethod {
		v2, _ := cmd.Flags().GetBool("v2")
		if v2 {
			return &methods.BlocksByRangeRPCv2
		} else {
			return &methods.BlocksByRangeRPCv1
		}
	}, func(cmd *cobra.Command, args []string) (interface{}, error) {
		startSlot, err := strconv.ParseUint(args[1], 0, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse start slot '%s'", args[1])
		}
		count, err := strconv.ParseUint(args[2], 0, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse count '%s'", args[1])
		}
		step, err := strconv.ParseUint(args[3], 0, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse step '%s'", args[1])
		}
		v2, err := cmd.Flags().GetBool("v2")
		if err != nil {
			return nil, err
		}
		if v2 {
			return &methods.BlocksByRangeReqV2{
				StartSlot:     methods.Slot(startSlot),
				Count:         count,
				Step:          step,
			}, nil
		} else {
			var root [32]byte
			if len(args) > 4 {
				root, err = parseRoot(args[4])
				if err != nil{
					return nil, err
				}
			}
			return &methods.BlocksByRangeReqV1{
				HeadBlockRoot: root,
				StartSlot:     methods.Slot(startSlot),
				Count:         count,
				Step:          step,
			}, nil
		}
	}, func(peerID peer.ID, chunkIndex uint64, responseCode uint8, readChunk func(dest interface{}) error) error {
		var data methods.SignedBeaconBlock
		if err := readChunk(&data); err != nil {
			return err
		}
		log.Infof("Block RPC response of peer %s: Slot: %d Parent root: %x Sig: %x", peerID.Pretty(), data.Message.Slot, data.Message.ParentRoot, data.Signature)
		return nil
	}, func(peerID peer.ID) {
		log.Infof("Blocks-by-range RPC responses of peer %s ended", peerID.Pretty())
	},
	)
	blocksByRangeCmd.Flags().Bool("v2", false, "To use v2 (no head root in request)")
	cmd.AddCommand(blocksByRangeCmd)

	fmtStatus := func(st *methods.Status) string {
		return fmt.Sprintf("head_fork_version: %x, finalized_root: %x, finalized_epoch: %d, head_root: %x, head_slot: %d",
		st.HeadForkVersion, st.FinalizedRoot, st.FinalizedEpoch, st.HeadRoot, st.HeadSlot)
	}
	var currentStatus methods.Status
	//respondingStatus := false

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Manage status RPC",
	}
	statusCmd.AddCommand(&cobra.Command{
		Use:   "view",
		Short: "Show current status",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			log.Infof("Current status: %s", fmtStatus(&currentStatus))
		},
	})
	parseStatus := func(args []string) (*methods.Status, error) {
		forkVersion, err := parseForkVersion(args[0])
		if err != nil {
			return nil, err
		}
		finalizedRoot, err := parseRoot(args[1])
		if err != nil {
			return nil, err
		}
		finalizedEpoch, err := strconv.ParseUint(args[2], 0, 64)
		if err != nil {
			return nil, err
		}
		headRoot, err := parseRoot(args[3])
		if err != nil {
			return nil, err
		}
		headSlot, err := strconv.ParseUint(args[4], 0, 64)
		if err != nil {
			return nil, err
		}
		return &methods.Status{
			HeadForkVersion: forkVersion,
			FinalizedRoot:   finalizedRoot,
			FinalizedEpoch:  methods.Epoch(finalizedEpoch),
			HeadRoot:        headRoot,
			HeadSlot:        methods.Slot(headSlot),
		}, nil
	}
	statusCmd.AddCommand(&cobra.Command{
		Use:   "set <head-fork-version> <finalized-root> <finalized-epoch> <head-root> <head-slot>",
		Short: "Change current status.",
		Args:  cobra.ExactArgs(5),
		Run: func(cmd *cobra.Command, args []string) {
			stat, err := parseStatus(args)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			currentStatus = *stat
			log.Infof("Set to status: %s", fmtStatus(&currentStatus))
		},
	})
	statusCmd.AddCommand(makeReqCmd(&cobra.Command{
		Use:   "req <peerID> [<head-fork-version> <finalized-root> <finalized-epoch> <head-root> <head-slot>]",
		Short: "Ask peer for status. Request with given status, or current status if not defined.",
		Args:  func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 && len(args) != 6 {
				return fmt.Errorf("accepts either 1 or 6 args, received %d", len(args))
			}
			return nil
		},
	}, func(cmd *cobra.Command) *reqresp.RPCMethod {
		return &methods.StatusRPCv1
	}, func(cmd *cobra.Command, args []string) (interface{}, error) {
		if len(args) != 1 {
			reqStatus, err := parseStatus(args[1:])
			if err != nil {
				return nil, err
			}
			return reqStatus, nil
		} else {
			return &currentStatus, nil
		}
	}, func(peerID peer.ID, chunkIndex uint64, responseCode uint8, readChunk func(dest interface{}) error) error {
		if chunkIndex > 0 {
			return fmt.Errorf("unexpected second Status response chunk with code %d from peer %s", responseCode, peerID.Pretty())
		}
		var data methods.Status
		if err := readChunk(&data); err != nil {
			return err
		}
		log.Infof("Status RPC response of peer %s: %s", peerID.Pretty(), fmtStatus(&data))
		return nil
	}, func(peerID peer.ID) {
		log.Infof("Status RPC responses of peer %s ended", peerID.Pretty())
	},
	))
	cmd.AddCommand(statusCmd)
	return cmd
}
