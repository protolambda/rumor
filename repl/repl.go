package repl

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
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
	"github.com/protolambda/rumor/addrutil"
	"github.com/protolambda/rumor/gossip"
	"github.com/protolambda/rumor/node"
	"github.com/protolambda/rumor/peering/dv5"
	"github.com/protolambda/rumor/peering/kad"
	"github.com/protolambda/rumor/peering/static"
	"github.com/protolambda/rumor/rpc/methods"
	"github.com/protolambda/rumor/rpc/reqresp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

type Repl struct {
	P2PHost host.Host

	PrivKey crypto.PrivKey

	IP      net.IP
	TcpPort uint16
	UdpPort uint16

	Dv5State Dv5State
	KadState KadState
	GossipState GossipState
	RPCState RPCState

	Log     logrus.FieldLogger
	Ctx     context.Context
	Cancel  context.CancelFunc
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
	return repl
}

func (r *Repl) Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "",
		Short: "A REPL for Eth2 networking.",
		Long:  `A REPL for Eth2 networking. For debugging and interacting with Eth2 network components.`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	cmd.AddCommand(
		r.InitHostCmd(),
		r.InitEnrCmd(),
		r.InitPeerCmd(),
		r.InitDv5Cmd(&r.Dv5State),
		r.InitKadCmd(&r.KadState),
		r.InitGossipCmd(&r.GossipState),
		r.InitRpcCmd(&r.RPCState),
	)
	return cmd
}

func (r *Repl) Host() host.Host {
	return r.P2PHost
}

func (r *Repl) Logger(logTopic string) logrus.FieldLogger {
	return r.Log.WithField("log_topic", logTopic)
}

func writeErrMsg(cmd *cobra.Command, format string, a ...interface{}) {
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), format+"\n", a...)
}

func writeErr(cmd *cobra.Command, err error) {
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), err.Error()+"\n")
}

func (r *Repl) NoHost(cmd *cobra.Command) bool {
	if r.P2PHost == nil {
		writeErrMsg(cmd, "REPL must have initialized Libp2p host. Try 'host start'")
		return true
	}
	return false
}

func (r *Repl) GetEnr() *enr.Record {
	priv := (*ecdsa.PrivateKey)(r.PrivKey.(*crypto.Secp256k1PrivateKey))
	return addrutil.MakeENR(r.IP, r.TcpPort, r.UdpPort, priv)
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
					priv, err := addrutil.ParsePrivateKey(privKeyStr)
					if err != nil {
						writeErr(cmd, err)
						return
					}
					r.PrivKey = (*crypto.Secp256k1PrivateKey)(priv)
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

	log := r.Logger("host")

	printEnr := func(cmd *cobra.Command) {
		enrStr, err := addrutil.EnrToString(r.GetEnr())
		if err != nil {
			writeErr(cmd, err)
			return
		}
		log.Infof("ENR address: %s", enrStr)
	}

	cmd.AddCommand(startCmd)
	{
		var ipStr string
		var tcpPort, udpPort uint16
		listenCmd := &cobra.Command{
			Use: "listen",
			Short: "Start listening on given address (see option flags).",
			Args: cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				if r.NoHost(cmd) {
					return
				}
				ip := net.ParseIP(ipStr)
				// hack to get a non-loopback address, to be improved.
				if ip == nil {
					ifaces, err := net.Interfaces()
					if err != nil {
						writeErr(cmd, err)
						return
					}
					for _, i := range ifaces {
						addrs, err := i.Addrs()
						if err != nil {
							writeErr(cmd, err)
							return
						}
						for _, addr := range addrs {
							var addrIP net.IP
							switch v := addr.(type) {
							case *net.IPNet:
								addrIP = v.IP
							case *net.IPAddr:
								addrIP = v.IP
							}
							if addrIP.IsGlobalUnicast() {
								ip = addrIP
							}
						}
					}
				}
				if ip == nil {
					writeErrMsg(cmd, "no IP found")
					return
				}
				ipScheme := "ip4"
				if ip4 := ip.To4(); ip4 == nil {
					ipScheme = "ip6"
				} else {
					ip = ip4
				}
				if udpPort == 0 {
					udpPort = tcpPort
				}
				log.Infof("ip: %s tcp: %d", ipScheme, tcpPort)
				mAddr, err := ma.NewMultiaddr(fmt.Sprintf("/%s/%s/tcp/%d", ipScheme, ip.String(), tcpPort))
				if err != nil {
					writeErrMsg(cmd, "could not construct multi addr: %v", err)
					return
				}
				if err := r.Host().Network().Listen(mAddr); err != nil {
					writeErr(cmd, err)
					return
				}
				r.IP = ip
				r.TcpPort = tcpPort
				r.UdpPort = udpPort
				printEnr(cmd)
			},
		}
		listenCmd.Flags().StringVar(&ipStr, "ip", "", "If no IP is specified, network interfaces are checked for one.")
		listenCmd.Flags().Uint16Var(&tcpPort, "tcp", 9000, "If no tcp port is specified, it defaults to 9000.")
		listenCmd.Flags().Uint16Var(&udpPort, "udp", 0, "If no udp port is specified (= 0), UDP equals TCP.")
		cmd.AddCommand(listenCmd)
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "view",
		Short: "View local peer ID, listening addresses, etc.",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(cmd) {
				return
			}
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

			printEnr(cmd)
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
			var enodeRes *enode.Node
			enodeRes, err = addrutil.EnrToEnode(rec, true)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			log.Infof("addr meta: seq: %d  node-ID: %s", enodeRes.Seq(), enodeRes.ID().String())
			log.Infof("enode addr: %s", enodeRes.URLv4())
			muAddr, err := addrutil.EnodeToMultiAddr(enodeRes)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			log.Infof("multi addr: %s", muAddr.String())
			log.Infof("ENR: %s", enodeRes.String())
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "make <ip> <tcp-port> <udp-port> <priv>",
		Short: "make an ENR. ENR is url-base64 (RFC 4648). Pubkey is raw hex encoded format",
		Args:  cobra.ExactArgs(4),
		Run: func(cmd *cobra.Command, args []string) {
			ip := net.ParseIP(args[0])
			if ip == nil {
				writeErrMsg(cmd, "could not parse ip: %s", args[0])
				return
			}
			tcpPort, err := strconv.ParseUint(args[1], 0, 16)
			if err != nil {
				writeErrMsg(cmd, "could not parse tcp port: %v", err)
				return
			}
			udpPort, err := strconv.ParseUint(args[2], 0, 16)
			if err != nil {
				writeErrMsg(cmd, "could not parse udp port: %v", err)
				return
			}
			priv, err := addrutil.ParsePrivateKey(args[3])
			if err != nil {
				writeErrMsg(cmd, "could not pubkey: %v", err)
				return
			}
			rec := addrutil.MakeENR(ip, uint16(tcpPort), uint16(udpPort), priv)
			enrStr, err := addrutil.EnrToString(rec)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			log.Infof("ENR address: %s", enrStr)
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
			if dv5Addr, err := addrutil.ParseEnodeAddr(addrStr); err != nil {
				muAddr, err = ma.NewMultiaddr(args[0])
				if err != nil {
					log.Info("addr not an enode or multi addr")
					writeErr(cmd, err)
					return
				}
			} else {
				muAddr, err = addrutil.EnodeToMultiAddr(dv5Addr)
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

type Dv5State struct {
	Dv5Node dv5.Discv5
	CloseDv5 context.CancelFunc
}

func (r *Repl) InitDv5Cmd(state *Dv5State) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dv5",
		Short: "Manage Ethereum Discv5",
	}
	log := r.Logger("discv5")

	noDv5 := func(cmd *cobra.Command) bool {
		if r.NoHost(cmd) {
			return true
		}
		if state.Dv5Node == nil {
			writeErrMsg(cmd, "REPL must have initialized discv5. Try 'dv5 start'")
			return true
		}
		return false
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "start [<bootstrap-addr> [...]]",
		Short: "Start discv5.",
		Long:  "Start discv5.",
		Args:  cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(cmd) {
				return
			}
			if r.IP == nil {
				writeErrMsg(cmd, "Host has no IP yet. Get with 'host listen'")
				return
			}
			if state.Dv5Node != nil {
				writeErrMsg(cmd, "Already have dv5 open at %s", state.Dv5Node.Self().String())
				return
			}
			bootNodes := make([]*enode.Node, 0, len(args))
			for i := 1; i < len(args); i++ {
				dv5Addr, err := addrutil.ParseEnodeAddr(args[i])
				if err != nil {
					writeErr(cmd, err)
					return
				}
				bootNodes = append(bootNodes, dv5Addr)
			}
			ctx, cancel := context.WithCancel(r.Ctx)
			var err error
			state.Dv5Node, err = dv5.NewDiscV5(ctx, r, r.IP, r.UdpPort, r.PrivKey, bootNodes)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			state.CloseDv5 = cancel
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
			state.CloseDv5()
			state.Dv5Node = nil
			log.Info("Stopped discv5")
		},
	})

	handleLookup := func(nodes []*enode.Node, connect bool) error {
		dv5AddrsOut := ""
		for i, v := range nodes {
			if i > 0 {
				dv5AddrsOut += "  "
			}
			dv5AddrsOut += v.String()
		}
		mAddrs, err := addrutil.EnodesToMultiAddrs(nodes)
		if err != nil {
			return err
		}
		mAddrsOut := ""
		for i, v := range mAddrs {
			if i > 0 {
				mAddrsOut += "  "
			}
			mAddrsOut += v.String()
		}
		log.Infof("addresses of nodes (%d): %s", len(mAddrs), mAddrsOut)
		if connect {
			log.Infof("connecting to nodes (with a 10 second timeout)")
			ctx, _ := context.WithTimeout(r.Ctx, time.Second*10)
			err := static.ConnectStaticPeers(ctx, r, mAddrs, func(info peer.AddrInfo, alreadyConnected bool) error {
				log.Infof("connected to peer from discv5 nodes: %s", info.String())
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "ping <target node: enode address or ENR (url-base64)>",
		Short: "Run discv5-ping",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			target, err := addrutil.ParseEnodeAddr(args[0])
			if err != nil {
				writeErr(cmd, err)
			}
			if err := state.Dv5Node.Ping(target); err != nil {
				writeErrMsg(cmd, "Failed to ping %s: %v", target.String(), err)
				return
			}
			log.Infof("Succesfully pinged %s: ", target.String())
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "resolve <target node: enode address or ENR (url-base64)>",
		Short: "Resolve target address and try to find latest record for it.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			target, err := addrutil.ParseEnodeAddr(args[0])
			if err != nil {
				writeErr(cmd, err)
			}
			resolved := state.Dv5Node.Resolve(target)
			if resolved != nil {
				writeErrMsg(cmd, "Failed to resolve %s, nil result", target.String())
				return
			}
			log.Infof("Succesfully resolved:   %s   -->  %s", target.String(), resolved.String())
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "get-enr <target node: enode address or ENR (url-base64)>",
		Short: "Resolve target address and try to find latest record for it.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			target, err := addrutil.ParseEnodeAddr(args[0])
			if err != nil {
				writeErr(cmd, err)
			}
			enrRes, err := state.Dv5Node.RequestENR(target)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			log.Infof("Succesfully got ENR for node:   %s   -->  %s", target.String(), enrRes.String())
		},
	})


	cmd.AddCommand(&cobra.Command{
		Use:   "random <N>",
		Short: "Get random list of N nodes.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			addrsLen, err := strconv.ParseUint(args[0], 0, 64)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			addrs := make([]*enode.Node, addrsLen)
			n := state.Dv5Node.ReadRandomNodes(addrs)
			if err := handleLookup(addrs[:n], false); err != nil {
				writeErr(cmd, err)
				return
			}
		},
	})

	connectLookup := false
	lookupCmd := &cobra.Command{
		Use:   "lookup [target node: hex node ID, enode address or ENR (url-base64)]",
		Short: "Get list of nearby multi addrs. If no target node is provided, then find nodes nearby to self.",
		Args:  cobra.RangeArgs(0, 1),
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			target := state.Dv5Node.Self().ID()
			if len(args) > 0 {
				if n, err := addrutil.ParseEnodeAddr(args[0]); err != nil {
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
					target = n.ID()
				}
			}
			res := state.Dv5Node.Lookup(target)
			if err := handleLookup(res, connectLookup); err != nil {
				writeErr(cmd, err)
				return
			}
		},
	}
	lookupCmd.Flags().BoolVar(&connectLookup, "connect", false, "Connect to the resulting nodes")
	cmd.AddCommand(lookupCmd)

	connectRandom := false
	randomCommand := &cobra.Command{
		Use:   "lookup-random",
		Short: "Get list of random multi addrs.",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			res := state.Dv5Node.LookupRandom()
			if err := handleLookup(res, connectRandom); err != nil {
				writeErr(cmd, err)
				return
			}
		},
	}
	randomCommand.Flags().BoolVar(&connectRandom, "connect", false, "Connect to the resulting nodes")
	cmd.AddCommand(randomCommand)

	cmd.AddCommand(&cobra.Command{
		Use:   "self",
		Short: "get local discv5 ENR",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			v, err := addrutil.EnrToString(r.GetEnr())
			if err != nil {
				writeErr(cmd, err)
				return
			}
			log.Infof("local ENR: %s", v)
			enodeAddr := state.Dv5Node.Self()
			log.Infof("local dv5 node (no TCP in ENR): %s", enodeAddr.String())
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "trace <bool>",
		Short: "be very verbose about discv5 interactions",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			v := strings.ToLower(args[0])
			tracing := v == "true" || v == "y" || v == "yes" || v == "on"
			state.Dv5Node.Trace(tracing)
			log.Infof("Set discv5 tracing mode to %v", tracing)
		},
	})
	return cmd
}

type KadState struct {
	KadNode kad.Kademlia
	CloseKad context.CancelFunc
}

func (r *Repl) InitKadCmd(state *KadState) *cobra.Command {
	noKad := func(cmd *cobra.Command) bool {
		if r.NoHost(cmd) {
			return true
		}
		if state.KadNode == nil {
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
			if state.KadNode != nil {
				writeErrMsg(cmd, "Already have a Kademlia DHT open")
				return
			}
			ctx, cancel := context.WithCancel(r.Ctx)
			var err error
			state.KadNode, err = kad.NewKademlia(ctx, r, protocol.ID(args[0]))
			if err != nil {
				writeErr(cmd, err)
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
			state.KadNode.RefreshTable(waitForRefreshResult)
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
			addrInfos, err := state.KadNode.FindPeersConnectedToPeer(ctx, peerID)
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
			addrInfo, err := state.KadNode.FindPeer(ctx, peerID)
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

type GossipState struct {
	GsNode gossip.GossipSub
	CloseGS context.CancelFunc
	Topics joinedTopics
	TopicLoggers topicLoggers
}

func (r *Repl) InitGossipCmd(state *GossipState) *cobra.Command {

	log := r.Logger("gossip")

	noGS := func(cmd *cobra.Command) bool {
		if r.NoHost(cmd) {
			return true
		}
		if state.GsNode == nil {
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
			if state.GsNode != nil {
				writeErrMsg(cmd, "Already started GossipSub")
				return
			}
			state.Topics = make(joinedTopics)
			ctx, cancel := context.WithCancel(r.Ctx)
			var err error
			state.GsNode, err = gossip.NewGossipSub(ctx, r)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			state.CloseGS = cancel
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
			state.GsNode = nil

			state.CloseGS()
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
			log.Infof("on %d topics:", len(state.Topics))
			for topic, _ := range state.Topics {
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
			if _, ok := state.Topics[topicName]; ok {
				writeErrMsg(cmd, "already on gossip topic %s", topicName)
				return
			}
			top, err := state.GsNode.JoinTopic(topicName)
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

			state.Topics[topicName] = top
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
			if top, ok := state.Topics[topicName]; !ok {
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
			state.GsNode.BlacklistPeer(peerID)
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
			if top, ok := state.Topics[topicName]; !ok {
				writeErrMsg(cmd, "not on gossip topic %s", topicName)
				return
			} else {
				err := top.Close()
				if err != nil {
					writeErr(cmd, err)
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
				writeErrMsg(cmd, "not on gossip topic %s", topicName)
				return
			} else {
				ctx, cancelLogger := context.WithCancel(r.Ctx)
				if err := logToFile(ctx, loggerName, top, outPath); err != nil {
					writeErr(cmd, err)
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
	return cmd
}

type RPCState struct {
	CurrentStatus methods.Status
}

func (r *Repl) InitRpcCmd(state *RPCState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rpc",
		Short: "Manage Eth2 RPC",
	}
	log := r.Logger("rpc")

	readOptionalComp := func(cmd *cobra.Command) (reqresp.Compression, error) {
		if compStr, err := cmd.Flags().GetString("compression"); err != nil {
			return nil, err
		} else {
			switch compStr {
			case "none", "", "false":
				// no compression
				return nil, nil
			case "snappy":
				return reqresp.SnappyCompression{}, nil
			default:
				return nil, fmt.Errorf("cannot recognize compression '%s'", compStr)
			}
		}
	}
	// TODO: stop responses command

	makeReqCmd := func(cmd *cobra.Command,
		rpcMethod func(cmd *cobra.Command) *reqresp.RPCMethod,
		mkReq func(cmd *cobra.Command, args []string) (reqresp.Request, error),
		onResp func(peerID peer.ID, chunkIndex uint64, readChunk func(dest interface{}) error) error,
		onClose func(peerID peer.ID),
	) *cobra.Command {
		cmd.Flags().String("compression", "none", "Optional compression. Try 'snappy' for streaming-snappy")
		cmd.Run = func(cmd *cobra.Command, args []string) {
			if r.NoHost(cmd) {
				return
			}
			sFn := reqresp.NewStreamFn(func(ctx context.Context, peerId peer.ID, protocolId protocol.ID) (network.Stream, error) {
				return r.P2PHost.NewStream(ctx, peerId, protocolId)
			}).WithTimeout(time.Second*10)
			ctx, _ := context.WithTimeout(r.Ctx, time.Second*10) // TODO add timeout option
			peerID, err := peer.Decode(args[0])
			if err != nil {
				writeErr(cmd, err)
				return
			}
			comp, err := readOptionalComp(cmd)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			req, err := mkReq(cmd, args)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			m := rpcMethod(cmd)
			lastRespChunkIndex := int64(-1)
			if err := m.RunRequest(ctx, sFn, peerID, comp, req,
				func(chunkIndex uint64, readChunk func(dest interface{}) error) error {
					log.Debugf("Received response chunk %d from peer %s", chunkIndex, peerID.Pretty())
					lastRespChunkIndex = int64(chunkIndex)
					return onResp(peerID, chunkIndex, readChunk)
				}, func(chunkIndex uint64, msg string) error {
					log.Errorf("request (protocol %s) to %s was turned down at chunk %d because of INVALID INPUT. Error message from server: %s", m.Protocol, peerID, chunkIndex, msg)
					return nil
				}, func(chunkIndex uint64, msg string) error {
					log.Errorf("request (protocol %s) to %s was turned down at chunk %d because of SERVER ERROR. Error message from server: %s", m.Protocol, peerID, chunkIndex, msg)
					return nil
				}, func() {
					log.Debugf("Responses of peer %s stopped after %d response chunks", peerID.Pretty(),lastRespChunkIndex+1)
					onClose(peerID)
				}); err != nil {
				writeErrMsg(cmd, "failed request: %v", err)
			}
		}
		return cmd
	}

	makeRespCmd := func(cmd *cobra.Command,
		rpcMethod func(cmd *cobra.Command) *reqresp.RPCMethod,
		handleReqFactory func(cmd *cobra.Command, args []string) (reqresp.ChunkedRequestHandler, error),
	) *cobra.Command {
		cmd.Flags().String("compression", "none", "Optional compression. Try 'snappy' for streaming-snappy")
		cmd.Flags().String("invalid-input", "", "If specified, an InvalidRequest(1) will be sent with the given message.")
		cmd.Flags().String("server-error", "", "If specified, an ServerError(2) will be sent with the given message.")
		cmd.Run = func(cmd *cobra.Command, args []string) {
			if cmd.Flags().Changed("invalid-input") && cmd.Flags().Changed("server-error") {
				writeErrMsg(cmd, "cannot write both invalid-input and server-error to response")
				return
			}
			if r.NoHost(cmd) {
				return
			}
			sCtxFn := func() context.Context {
				ctx, _ := context.WithTimeout(r.Ctx, time.Second*10) // TODO add timeout option
				return ctx
			}
			comp, err := readOptionalComp(cmd)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			m := rpcMethod(cmd)
			handleReq, err := handleReqFactory(cmd, args)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			handleReqWrap := func(ctx context.Context, peerId peer.ID, request reqresp.Request, respChunk reqresp.WriteSuccessChunkFn, respChunkInvalidInput reqresp.WriteMsgFn, respChunkServerError reqresp.WriteMsgFn) error {
				log.Debugf("Got request from %s, protocol %s: %s", peerId.Pretty(), m.Protocol, request.String())
				respChunkWrap := func(data interface{}) error {
					log.Debugf("Responding SUCCESS to peer %s with data: %v", peerId.Pretty(), data)
					return respChunk(data)
				}
				respInvalidInputWrap := func(msg string) error {
					log.Debugf("Responding INVALID INPUT to peer %s with message: '%s'", peerId.Pretty(), msg)
					return respChunkInvalidInput(msg)
				}
				respServerErrWrap := func(msg string) error {
					log.Debugf("Responding SERVER ERROR to peer %s with message: '%s'", peerId.Pretty(), msg)
					return respChunkInvalidInput(msg)
				}
				if cmd.Flags().Changed("invalid-input") {
					v, _ := cmd.Flags().GetString("invalid-input")
					return respChunkInvalidInput(v)
				}
				if cmd.Flags().Changed("server-error") {
					v, _ := cmd.Flags().GetString("server-error")
					return respChunkServerError(v)
				}
				return handleReq(ctx, peerId, request, respChunkWrap, respInvalidInputWrap, respServerErrWrap)
			}
			onInvalidInput := func(ctx context.Context, peerId peer.ID, err error) {
				log.Warnf("Got invalid input from %s, protocol %s, err: %v", peerId.Pretty(), m.Protocol, err)
			}
			onServerError := func(ctx context.Context, peerId peer.ID, err error) {
				log.Warnf("Server error on request from %s, protocol %s, err: %v", peerId.Pretty(), m.Protocol, err)
			}
			streamHandler, err := m.MakeStreamHandler(sCtxFn, comp, handleReqWrap, onInvalidInput, onServerError)
			if err != nil {
				writeErr(cmd, err)
				return
			}
			r.P2PHost.SetStreamHandler(m.Protocol, streamHandler)
		}
		return cmd
	}

	responseNotImplemented := func(cmd *cobra.Command, args []string) (handler reqresp.ChunkedRequestHandler, err error) {
		return func(ctx context.Context, peerId peer.ID, request reqresp.Request, respChunk reqresp.WriteSuccessChunkFn, onInvalidInput reqresp.WriteMsgFn, onServerErr reqresp.WriteMsgFn) error {
			log.Infof("Received request: %s", request.String())
			return fmt.Errorf("response-type is not implemented to make success responses. Ignoring request and closing stream")
		}, nil
	}

	goodbyeCmd := &cobra.Command{
		Use:   "goodbye",
		Short: "Manage goodbye RPC",
	}
	goodbyeCmd.AddCommand(makeReqCmd(&cobra.Command{
		Use:   "req <peerID> <code>",
		Short: "Send a goodbye to a peer, optionally disconnecting the peer after sending the Goodbye.",
		Args: cobra.ExactArgs(2),
	}, func(cmd *cobra.Command) *reqresp.RPCMethod {
		return &methods.GoodbyeRPCv1
	}, func(cmd *cobra.Command, args []string) (reqresp.Request, error) {
			v, err := strconv.ParseUint(args[1], 0, 64)
			if err != nil {
				return nil, fmt.Errorf("failed to parse Goodbye code '%s'", args[1])
			}
			req := methods.Goodbye(v)
			return &req, nil
		}, func(peerID peer.ID, chunkIndex uint64, readChunk func(dest interface{}) error) error {
			if chunkIndex > 0 {
				return fmt.Errorf("unexpected second Goodbye response chunk from peer %s", peerID.Pretty())
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
	goodbyeCmd.AddCommand(makeRespCmd(&cobra.Command{
		Use:   "resp",
		Short: "Respond to peers.",
		Args:  cobra.NoArgs,
	}, func(cmd *cobra.Command) *reqresp.RPCMethod {
		return &methods.GoodbyeRPCv1
	}, responseNotImplemented))
	cmd.AddCommand(goodbyeCmd)

	parseRoot := func(v string) ([32]byte, error) {
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

	blocksByRangeCmd := &cobra.Command{
		Use:   "blocks-by-range",
		Short: "Manage blocks-by-range RPC",
	}
	chooseBlocksByRangeVersion := func(cmd *cobra.Command) *reqresp.RPCMethod {
		v2, _ := cmd.Flags().GetBool("v2")
		if v2 {
			return &methods.BlocksByRangeRPCv2
		} else {
			return &methods.BlocksByRangeRPCv1
		}
	}
	blocksByRangeReqCmd := makeReqCmd(&cobra.Command{
		Use:   "req <peerID> <start-slot> <count> <step> [head-root-hex]",
		Short: "Get blocks by range from a peer. The head-root is optional, and defaults to zeroes. Use --v2 for no head-root.",
		Args: cobra.RangeArgs(4, 5),
	}, chooseBlocksByRangeVersion, func(cmd *cobra.Command, args []string) (reqresp.Request, error) {
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
	}, func(peerID peer.ID, chunkIndex uint64, readChunk func(dest interface{}) error) error {
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
	blocksByRangeReqCmd.Flags().Bool("v2", false, "To use v2 (no head root in request)")
	blocksByRangeCmd.AddCommand(blocksByRangeReqCmd)

	blocksByRangeRespCmd := makeRespCmd(&cobra.Command{
		Use:   "resp",
		Short: "Respond to peers.",
		Args:  cobra.NoArgs,
	}, chooseBlocksByRangeVersion, responseNotImplemented)
	blocksByRangeRespCmd.Flags().Bool("v2", false, "To use v2 (no head root in request)")

	blocksByRangeCmd.AddCommand(blocksByRangeRespCmd)

	cmd.AddCommand(blocksByRangeCmd)

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Manage status RPC",
	}
	statusCmd.AddCommand(&cobra.Command{
		Use:   "view",
		Short: "Show current status",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			log.Infof("Current status: %s", state.CurrentStatus.String())
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
			state.CurrentStatus = *stat
			log.Infof("Set to status: %s", state.CurrentStatus.String())
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
	}, func(cmd *cobra.Command, args []string) (reqresp.Request, error) {
		if len(args) != 1 {
			reqStatus, err := parseStatus(args[1:])
			if err != nil {
				return nil, err
			}
			return reqStatus, nil
		} else {
			return &state.CurrentStatus, nil
		}
	}, func(peerID peer.ID, chunkIndex uint64, readChunk func(dest interface{}) error) error {
		log.Infof("status resp %d", chunkIndex)
		if chunkIndex > 0 {
			return fmt.Errorf("unexpected second Status response chunk from peer %s", peerID.Pretty())
		}
		var data methods.Status
		if err := readChunk(&data); err != nil {
			return err
		}
		log.Infof("Status RPC response of peer %s: %s", peerID.Pretty(), data.String())
		return nil
	}, func(peerID peer.ID) {
		log.Infof("Status RPC responses of peer %s ended", peerID.Pretty())
	},
	))
	statusCmd.AddCommand(makeRespCmd(&cobra.Command{
		Use:   "resp",
		Short: "Respond to peers with current status.",
		Args:  cobra.NoArgs,
	}, func(cmd *cobra.Command) *reqresp.RPCMethod {
		return &methods.StatusRPCv1
	}, func(cmd *cobra.Command, args []string) (handler reqresp.ChunkedRequestHandler, err error) {
		return func(ctx context.Context, peerId peer.ID, request reqresp.Request, respChunk reqresp.WriteSuccessChunkFn, onInvalidInput reqresp.WriteMsgFn, onServerErr reqresp.WriteMsgFn) error {
			return respChunk(&state.CurrentStatus)
		}, nil
	}))

	cmd.AddCommand(statusCmd)
	return cmd
}
