package actor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/network"
	mplex "github.com/libp2p/go-libp2p-mplex"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	secio "github.com/libp2p/go-libp2p-secio"
	yamux "github.com/libp2p/go-libp2p-yamux"
	"github.com/libp2p/go-tcp-transport"
	ws "github.com/libp2p/go-ws-transport"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"net"
	"strings"
	"time"
)

func (r *Actor) InitHostCmd(ctx context.Context, log logrus.FieldLogger) *cobra.Command {
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
	var natEnabled bool

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the host node. See flags for security, transport, mux etc. options",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			r.hLock.Lock()
			defer r.hLock.Unlock()
			if r.P2PHost != nil {
				log.Error("Already have a host open.")
				return
			}
			{
				if privKeyStr == "" { // generate new private key if non was specified
					var err error
					r.PrivKey, _, err = crypto.GenerateKeyPairWithReader(crypto.Secp256k1, -1, rand.Reader)
					if err != nil {
						log.Error(err)
						return
					}
					p, err := r.PrivKey.Raw()
					if err != nil {
						log.Error(err)
						return
					}
					log.WithField("priv", hex.EncodeToString(p)).Info("Generated random Secp256k1 private key")
				} else {
					priv, err := addrutil.ParsePrivateKey(privKeyStr)
					if err != nil {
						log.Error(err)
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
					log.Errorf("could not recognize transport %s", v)
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
					log.Errorf("could not recognize mux %s", v)
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
					log.Errorf("could not recognize security %s", securityStr)
					return
				}
			}

			if natEnabled {
				hostOptions = append(hostOptions, libp2p.NATPortMap())
			}

			if relayEnabled {
				hostOptions = append(hostOptions, libp2p.EnableRelay())
			}

			hostOptions = append(hostOptions,
				libp2p.Identity(r.PrivKey),
				libp2p.Peerstore(pstoremem.NewPeerstore()), // TODO: persist peerstore?
				libp2p.ConnectionManager(connmgr.NewConnManager(loPeers, hiPeers, time.Millisecond*time.Duration(gracePeriodMs))),
			)
			// Not the command ctx, we want the host to stay open after the command.
			h, err := libp2p.New(r.ActorCtx, hostOptions...)
			if err != nil {
				log.Error(err)
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
	startCmd.Flags().BoolVar(&natEnabled, "nat", true, "enable nat address discovery (upnp/pmp)")
	startCmd.Flags().IntVar(&loPeers, "lo-peers", 15, "low-water for connection manager to trim peer count to")
	startCmd.Flags().IntVar(&hiPeers, "hi-peers", 20, "high-water for connection manager to trim peer count from")
	startCmd.Flags().IntVar(&gracePeriodMs, "peer-grace-period", 20_000, "Time in milliseconds to grace a peer from being trimmed")

	cmd.AddCommand(startCmd)

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the host node.",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			r.hLock.Lock()
			defer r.hLock.Unlock()
			if r.P2PHost == nil {
				log.Error("No host was open.")
				return
			}
			err := r.P2PHost.Close()
			if err != nil {
				log.Error(err)
			} else {
				log.Info("Successfully closed host")
			}
			r.P2PHost = nil
		},
	}
	cmd.AddCommand(stopCmd)

	printEnr := func(cmd *cobra.Command) {
		enrStr, err := addrutil.EnrToString(r.GetEnr())
		if err != nil {
			log.Error(err)
			return
		}
		log.WithField("enr", enrStr).Info("ENR") // TODO put all data in fields
	}

	{
		var ipStr string
		var tcpPort, udpPort uint16
		listenCmd := &cobra.Command{
			Use:   "listen",
			Short: "Start listening on given address (see option flags).",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				h, hasHost := r.Host(log)
				if !hasHost {
					return
				}
				ip := net.ParseIP(ipStr)
				// hack to get a non-loopback address, to be improved.
				if ip == nil {
					ifaces, err := net.Interfaces()
					if err != nil {
						log.Error(err)
						return
					}
					for _, i := range ifaces {
						addrs, err := i.Addrs()
						if err != nil {
							log.Error(err)
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
					log.Error("no IP found")
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
					log.Errorf("could not construct multi addr: %v", err)
					return
				}
				if err := h.Network().Listen(mAddr); err != nil {
					log.Error(err)
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
			h, hasHost := r.Host(log)
			if !hasHost {
				return
			}
			log.WithField("peer_id", h.ID()).Info("Peer ID")
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
	{
		listenF := func(net network.Network, addr ma.Multiaddr) {
			log.WithFields(logrus.Fields{"event": "listen_open", "addr": addr.String()}).Debug("opened network listener")
		}
		listenCloseF := func(net network.Network, addr ma.Multiaddr) {
			log.WithFields(logrus.Fields{"event": "listen_close", "addr": addr.String()}).Debug("closed network listener")
		}

		connectedF := func(net network.Network, conn network.Conn) {
			log.WithFields(logrus.Fields{
				"event": "connection_open", "peer": conn.RemotePeer().String(),
				"direction": fmtDirection(conn.Stat().Direction),
			}).Debug("new peer connection")
		}
		disconnectedF := func(net network.Network, conn network.Conn) {
			log.WithFields(logrus.Fields{
				"event": "connection_close", "peer": conn.RemotePeer().String(),
				"direction": fmtDirection(conn.Stat().Direction),
			}).Debug("peer disconnected")
		}

		openedStreamF := func(net network.Network, str network.Stream) {
			log.WithFields(logrus.Fields{
				"event": "stream_open", "peer": str.Conn().RemotePeer().String(),
				"direction": fmtDirection(str.Stat().Direction),
				"protocol":  str.Protocol(),
			}).Debug("opened stream")
		}
		closedStreamF := func(net network.Network, str network.Stream) {
			log.WithFields(logrus.Fields{
				"event": "stream_close", "peer": str.Conn().RemotePeer().String(),
				"direction": fmtDirection(str.Stat().Direction),
				"protocol":  str.Protocol(),
			}).Debug("closed stream")
		}

		cmd.AddCommand(&cobra.Command{
			Use:   "notify <event-types>...",
			Short: "Get notified of specific events, as long as the command runs.",
			Long: `
Network event notifications. 
Valid event types: 
 - listen (listen_open listen_close)
 - connection (connection_open connection_close) 
 - stream (stream_open stream_close)
 - all

Notification logs will have keys: "event" - one of the above detailed event types, e.g. listen_close.
- "peer": peer ID
- "direction": "inbound"/"outbound"/"unknown", for connections and streams
- "extra": stream/connection extra data
- "protocol": protocol ID for streams.
`,
			Args: cobra.MinimumNArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				h, hasHost := r.Host(log)
				if !hasHost {
					return
				}
				bundle := &network.NotifyBundle{}
				for _, notifyType := range args {
					notifyType = strings.TrimSpace(notifyType)
					if notifyType == "" {
						continue
					}
					switch notifyType {
					case "listen_open":
						bundle.ListenF = listenF
					case "listen_close":
						bundle.ListenCloseF = listenCloseF
					case "connection_open":
						bundle.ConnectedF = connectedF
					case "connection_close":
						bundle.DisconnectedF = disconnectedF
					case "stream_open":
						bundle.OpenedStreamF = openedStreamF
					case "stream_close":
						bundle.ClosedStreamF = closedStreamF
					case "listen":
						bundle.ListenF = listenF
						bundle.ListenCloseF = listenCloseF
					case "connection":
						bundle.ConnectedF = connectedF
						bundle.DisconnectedF = disconnectedF
					case "stream":
						bundle.OpenedStreamF = openedStreamF
						bundle.ClosedStreamF = closedStreamF
					case "all":
						bundle.ListenF = listenF
						bundle.ListenCloseF = listenCloseF
						bundle.ConnectedF = connectedF
						bundle.DisconnectedF = disconnectedF
						bundle.OpenedStreamF = openedStreamF
						bundle.ClosedStreamF = closedStreamF
					default:
						log.Errorf("unrecognized notification type: %s", notifyType)
						return
					}
				}
				h.Network().Notify(bundle)
				<-ctx.Done()
				h.Network().StopNotify(bundle)
			},
		})
	}
	return cmd
}

func fmtDirection(d network.Direction) string {
	switch d {
	case network.DirInbound:
		return "inbound"
	case network.DirOutbound:
		return "outbound"
	case network.DirUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}
