package repl

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"eth2-lurk/node"
	"eth2-lurk/peering/static"
	"fmt"
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
	PrivLibp2pRSA crypto.PrivKey
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
		Use:   "eth2-net-repl",
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
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), format + "\n", a)
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
		Run: func(cmd *cobra.Command, args []string) {
			if r.P2PHost != nil {
				writeErrMsg(cmd, "Already have a host open.")
				return
			}
			{
				if privKeyStr == "" { // generate new private key if non was specified
					var err error
					r.PrivLibp2pRSA, _, err = crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, rand.Reader)
					if err != nil {
						writeErr(cmd, err)
						return
					}
				} else {
					privKeyBytes, err := hex.DecodeString(privKeyStr)
					if err != nil {
						writeErrMsg(cmd, "cannot parse private key, expected hex string (without 0x)")
						return
					}
					r.PrivLibp2pRSA, err = crypto.UnmarshalRsaPrivateKey(privKeyBytes)
					if err != nil {
						writeErrMsg(cmd, "cannot parse private key, invalid RSA key")
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
				libp2p.Identity(r.PrivLibp2pRSA),
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
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(cmd) {
				return
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
		Use:   "list <all | connected>",
		Short: "List peers in peerstore",
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
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
		Run: func(cmd *cobra.Command, args []string) {
			ctx, _ := context.WithTimeout(context.Background(), time.Second * 2)
			r.P2PHost.ConnManager().TrimOpenConns(ctx)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "connect <multi addr> [tag]",
		Short: "Connect to peer",
		Args: cobra.RangeArgs(1, 2),
		Run: func(cmd *cobra.Command, args []string) {
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
		Run: func(cmd *cobra.Command, args []string) {
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
		Use:   "addrs <peerID>",
		Short: "View known addresses of <peerID>",
		Run: func(cmd *cobra.Command, args []string) {
			peerID, err := peer.Decode(args[0])
			if err != nil {
				writeErr(cmd, err)
				return
			}
			log := r.Logger("peer-addrs")
			for i, a := range r.P2PHost.Peerstore().Addrs(peerID) {
				log.Infof("%s addr #%d: %s", peerID.Pretty(), i, a.String())
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
	cmd.AddCommand(&cobra.Command{
		Use:   "start [--addr=UDP-addr or --enr=ENR-addr] [--priv-key=priv key]",
		Short: "Start discv5 discovery",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "stop [--addr=... or --enr=... or --pubkey=...]",
		Short: "Stop discv5 discovery on given address",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "search",
		Short: "Actively query for new discv5 nodes",
		Run: func(cmd *cobra.Command, args []string) {

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

