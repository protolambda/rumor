package repl

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"eth2-lurk/node"
	"eth2-lurk/peering/dv5"
	"eth2-lurk/peering/static"
	"fmt"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	mplex "github.com/libp2p/go-libp2p-mplex"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	secio "github.com/libp2p/go-libp2p-secio"
	yamux "github.com/libp2p/go-libp2p-yamux"
	"github.com/libp2p/go-tcp-transport"
	ws "github.com/libp2p/go-ws-transport"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"strings"
	"time"
)

type Repl struct {
	P2PHost host.Host
	PrivKey crypto.PrivKey
	Log     logrus.FieldLogger
	Ctx     context.Context
	Cancel  func()
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
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), format + "\n", a...)
}

func writeErr(cmd *cobra.Command, err error) {
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), err.Error() + "\n")
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
		Use:   "start [--priv-key=...] [--security=<secio|tls|noise>] [--mux=yamux,mplex] [--transports=tcp,ws]",
		Short: "Start the host node",
		Args: cobra.NoArgs,
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
		Args: cobra.ArbitraryArgs,
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
		Args: cobra.NoArgs,
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
	cmd.AddCommand(&cobra.Command{
		Use:   "view <enr url-base64 (RFC 4648)>",
		Short: "view ENR contents",
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			data, err := base64.RawURLEncoding.DecodeString(args[0])
			if err != nil {
				writeErr(cmd, err)
				return
			}
			var record enr.Record
			if err := rlp.Decode(bytes.NewReader(data), &record); err != nil {
				writeErr(cmd, err)
				return
			}
			// TODO print record details
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "from <multi addr>",
		Short: "create ENR (encoded in url-base64 (RFC 4648)) from multi addr",
		Run: func(cmd *cobra.Command, args []string) {

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
		Use:   "list [all | connected]",
		Short: "List peers in peerstore. Defaults to connected only.",
		Args: cobra.ArbitraryArgs,
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
			log.Infof("%d peers:", len(peers))
			for i, p := range peers {
				log.Infof("%4d: %s", i, r.P2PHost.Peerstore().PeerInfo(p).String())
			}
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "trim",
		Short: "Trim peers (2 second time allowance)",
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(cmd) {
				return
			}
			ctx, _ := context.WithTimeout(context.Background(), time.Second * 2)
			r.P2PHost.ConnManager().TrimOpenConns(ctx)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "connect <multi addr> [tag]",
		Short: "Connect to peer",
		Args: cobra.RangeArgs(1, 2),
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(cmd) {
				return
			}
			muAddr, err := ma.NewMultiaddr(args[0])
			if err != nil {
				writeErr(cmd, err)
				return
			}
			addrInfo, err := peer.AddrInfoFromP2pAddr(muAddr)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			ctx, _ := context.WithTimeout(context.Background(), time.Second * 5)
			if err := r.P2PHost.Connect(ctx, *addrInfo); err != nil {
				writeErr(cmd, err)
				return
			}
			log := r.Logger("connect-peer")
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
		Args: cobra.ExactArgs(1),
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
		Args: cobra.ExactArgs(2),
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
		Args: cobra.ExactArgs(2),
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
		Args: cobra.RangeArgs(0, 1),
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

func (r *Repl) InitDv5Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dv5",
		Short: "Manage Ethereum Discv5",
	}

	var dv5Node dv5.Discv5
	var closeDv5 func()
	parseDv5Addr := func(v string) (*discv5.Node, error) {
		if strings.HasPrefix(v, "enode://") {
			addr := new(discv5.Node)
			err := addr.UnmarshalText([]byte(v))
			if err != nil {
				return nil, err
			}
			return addr, nil
		} else {
			enrAddr, err := dv5.ParseEnr(v)
			if err != nil {
				return nil, err
			}
			// TODO: warn if no "eth2" key in ENR record.
			enodeAddr, err := dv5.EnrToEnode(enrAddr, true)
			if err != nil {
				return nil, err
			}
			return dv5.EnodeToDiscv5Node(enodeAddr)
		}
	}
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
		Use:   "start <UDP address with port>",
		Short: "Start discv5.",
		Long: "Start discv5.",
		Args: cobra.ExactArgs(1),
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
		Use:   "bootstrap <Dv5-addr or ENR-addr>",
		Short: "Bootstrap discv5 by connecting to the given discv5 node.",
		Long: "Dv5 addr format example: 'enode://<hex node id>@10.3.58.6:30303?discport=30301', enr is url-base64 encoded.",
		Args: cobra.RangeArgs(1, 2),
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
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop discv5",
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			closeDv5()
			dv5Node = nil
			r.Logger("dv5").Info("Stopped discv5")
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
		Args: cobra.RangeArgs(0, 1),
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
			log.Infof("addresses of nearby nodes: %s", mAddrsOut)
			if connectNearby {
				log.Infof("connecting to nearby nodes (with a 10 second timeout)")
				ctx, _ := context.WithTimeout(r.Ctx, time.Second * 10)
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
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			r.Logger("discv5").Infof("local dv5 node: %s  UDP address: %s", dv5Node.Self().String(), dv5Node.UDPAddress().String())
		},
	})
	return cmd
}

func (r *Repl) InitKadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kad",
		Short: "Manage Libp2p Kademlia DHT",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "start <protocol ID>",
		Short: "Go onto the given Kademlia DHT, if known by any connected peer (connect to bootnode first)",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "refresh [--wait]",
		Short: "Refresh the Kademlia table. Optionally wait for it to complete.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	return cmd
}

func (r *Repl) InitGossipCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gossip",
		Short: "Manage Libp2p GossipSub",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List joined gossip topics",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "join <topic>",
		Short: "Join a gossip topic. Propagate anything.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "lurk <topic>",
		Short: "Lurk a gossip topic. Propagate nothing.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "leave <topic>",
		Short: "Leave a gossip topic.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "log <topic> <output file path>",
		Short: "Log the messages of a gossip topic to a file. 1 hex-encoded message per line. Join/lurk a topic first.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	return cmd
}

func (r *Repl) InitRpcCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "rpc",
		Short: "Manage Eth2 RPC",
	}
	cmd.AddCommand(&cobra.Command{
		Use: "list",
		Short: "List active RPC listeners",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "log-req <method name> [compression]",
		Short: "Log requests to a file",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "goodbye <peerID> <code> [compression] [--disconnect]",
		Short: "Send a goodbye to a peer, optionally disconnecting the peer after sending the Goodbye.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	return cmd
}

