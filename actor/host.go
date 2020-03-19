package actor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	mplex "github.com/libp2p/go-libp2p-mplex"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	secio "github.com/libp2p/go-libp2p-secio"
	yamux "github.com/libp2p/go-libp2p-yamux"
	"github.com/libp2p/go-tcp-transport"
	ws "github.com/libp2p/go-ws-transport"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/protolambda/rumor/addrutil"
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
					p, err := crypto.MarshalPrivateKey(r.PrivKey)
					if err != nil {
						log.Error(err)
						return
					}
					log.Infof("Generated random Secp256k1 private key: %s", hex.EncodeToString(p))
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
			h, err := libp2p.New(r.Ctx, hostOptions...)
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

	printEnr := func(cmd *cobra.Command) {
		enrStr, err := addrutil.EnrToString(r.GetEnr())
		if err != nil {
			log.Error(err)
			return
		}
		log.WithField("enr", enrStr).Info("ENR")  // TODO put all data in fields
	}

	cmd.AddCommand(startCmd)
	{
		var ipStr string
		var tcpPort, udpPort uint16
		listenCmd := &cobra.Command{
			Use:   "listen",
			Short: "Start listening on given address (see option flags).",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				if r.NoHost(log) {
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
				if err := r.Host().Network().Listen(mAddr); err != nil {
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
			if r.NoHost(log) {
				return
			}
			h := r.Host()
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
	return cmd
}
