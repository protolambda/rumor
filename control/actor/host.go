package actor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/sirupsen/logrus"
	"net"
	"strings"
	"time"
)

type HostCmd struct {
	*Actor `ask:"-"`
	log    logrus.FieldLogger
}

func (c *HostCmd) Help() string {
	return "Manage the libp2p host"
}

func (c *HostCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "start":
		cmd = DefaultHostStartCmd(c.Actor, c.log)
	case "stop":
		cmd = (*HostStopCmd)(DefaultBasicCmd(c.Actor, c.log))
	case "view":
		cmd = (*HostViewCmd)(DefaultBasicCmd(c.Actor, c.log))
	case "listen":
		cmd = DefaultHostListenCmd(c.Actor, c.log)
	case "event":
		cmd = (*HostEventCmd)(DefaultBasicCmd(c.Actor, c.log))
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

type HostStartCmd struct {
	*Actor           `ask:"-"`
	log              logrus.FieldLogger
	PrivKeyStr       string   `ask:"--priv" help:"hex-encoded RSA private key for libp2p host. Random if none is specified."`
	TransportsStrArr []string `ask:"--transport" help:"Transports to use. Options: tcp, ws"`
	MuxStrArr        []string `ask:"--mux" help:"Multiplexers to use"`
	SecurityStr      string   `ask:"--security" help:"Security to use. Options: secio, none"`
	RelayEnabled     bool     `ask:"--relay" help:"enable relayer functionality"`
	LoPeers          int      `ask:"--lo-peers" help:"low-water for connection manager to trim peer count to"`
	HiPeers          int      `ask:"--hi-peers" help:"high-water for connection manager to trim peer count from"`
	GracePeriodMs    int      `ask:"--peer-grace-period" help:"Time in milliseconds to grace a peer from being trimmed"`
	NatEnabled       bool     `ask:"--nat" help:"enable nat address discovery (upnp/pmp)"`
}

func DefaultHostStartCmd(a *Actor, log logrus.FieldLogger) *HostStartCmd {
	return &HostStartCmd{
		Actor:            a,
		log:              log,
		PrivKeyStr:       "",
		TransportsStrArr: []string{"tcp"},
		MuxStrArr:        []string{"yamux", "mplex"},
		SecurityStr:      "secio",
		RelayEnabled:     false,
		LoPeers:          15,
		HiPeers:          20,
		GracePeriodMs:    20_000,
		NatEnabled:       true,
	}
}

func (c *HostStartCmd) Help() string {
	return "Start the host node. See flags for security, transport, mux etc. options"
}

func (c *HostStartCmd) Run(ctx context.Context, args ...string) error {
	c.hLock.Lock()
	defer c.hLock.Unlock()
	if c.P2PHost != nil {
		return errors.New("Already have a host open.")
	}
	{
		if c.PrivKeyStr == "" { // generate new private key if non was specified
			var err error
			c.PrivKey, _, err = crypto.GenerateKeyPairWithReader(crypto.Secp256k1, -1, rand.Reader)
			if err != nil {
				return err
			}
			p, err := c.PrivKey.Raw()
			if err != nil {
				return err
			}
			c.log.WithField("priv", hex.EncodeToString(p)).Info("Generated random Secp256k1 private key")
		} else {
			priv, err := addrutil.ParsePrivateKey(c.PrivKeyStr)
			if err != nil {
				return err
			}
			c.PrivKey = (*crypto.Secp256k1PrivateKey)(priv)
		}
	}
	hostOptions := make([]libp2p.Option, 0)

	for _, v := range c.TransportsStrArr {
		v = strings.ToLower(strings.TrimSpace(v))
		switch v {
		case "tcp":
			hostOptions = append(hostOptions, libp2p.Transport(tcp.NewTCPTransport))
		case "ws":
			hostOptions = append(hostOptions, libp2p.Transport(ws.New))
		default:
			return fmt.Errorf("could not recognize transport %s", v)
		}
	}

	for _, v := range c.MuxStrArr {
		v = strings.ToLower(strings.TrimSpace(v))
		switch v {
		case "yamux":
			hostOptions = append(hostOptions, libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport))
		case "mplex":
			hostOptions = append(hostOptions, libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport))
		default:
			return fmt.Errorf("could not recognize mux %s", v)
		}
	}

	{
		switch c.SecurityStr {
		case "none":
			// no security, for debugging etc.
		case "secio":
			hostOptions = append(hostOptions, libp2p.Security(secio.ID, secio.New))
		default:
			return fmt.Errorf("could not recognize security %s", c.SecurityStr)
		}
	}

	if c.NatEnabled {
		hostOptions = append(hostOptions, libp2p.NATPortMap())
	}

	if c.RelayEnabled {
		hostOptions = append(hostOptions, libp2p.EnableRelay())
	}

	hostOptions = append(hostOptions,
		libp2p.Identity(c.PrivKey),
		libp2p.Peerstore(pstoremem.NewPeerstore()), // TODO: persist peerstore?
		libp2p.ConnectionManager(connmgr.NewConnManager(c.LoPeers, c.HiPeers, time.Millisecond*time.Duration(c.GracePeriodMs))),
	)
	// Not the command ctx, we want the host to stay open after the command.
	h, err := libp2p.New(c.ActorCtx, hostOptions...)
	if err != nil {
		return err
	}
	c.P2PHost = h
	return nil
}

type HostStopCmd struct {
	*HostCmd `ask:"-"`
}

func (c *HostStopCmd) Help() string {
	return "Stop the host node."
}

func (c *HostStopCmd) Run(ctx context.Context, args ...string) error {
	c.hLock.Lock()
	defer c.hLock.Unlock()
	if c.P2PHost == nil {
		return errors.New("No host was open.")
	}
	err := c.P2PHost.Close()
	if err != nil {
		return err
	} else {
		c.log.Info("Successfully closed host")
	}
	c.P2PHost = nil
	return nil
}

type HostListenCmd struct {
	*Actor `ask:"-"`
	log    logrus.FieldLogger
	IP     net.IP `ask:"--ip" help:"If no IP is specified, network interfaces are checked for one."`

	TcpPort uint16 `ask:"--tcp" help:"If no tcp port is specified, it defaults to 9000."`
	UdpPort uint16 `ask:"--udp" help:"If no udp port is specified (= 0), UDP equals TCP."`
}

func DefaultHostListenCmd(a *Actor, log logrus.FieldLogger) *HostListenCmd {
	return &HostListenCmd{
		Actor:   a,
		log:     log,
		IP:      nil,
		TcpPort: 9000,
		UdpPort: 9000}
}

func (c *HostListenCmd) Help() string {
	return "Start listening on given address (see option flags)."
}

func (c *HostListenCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	// hack to get a non-loopback address, to be improved.
	if c.IP == nil {
		ifaces, err := net.Interfaces()
		if err != nil {
			return err
		}
		for _, i := range ifaces {
			addrs, err := i.Addrs()
			if err != nil {
				return err
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
					c.IP = addrIP
				}
			}
		}
	}
	if c.IP == nil {
		return errors.New("no IP found")
	}
	ipScheme := "ip4"
	if ip4 := c.IP.To4(); ip4 == nil {
		ipScheme = "ip6"
	} else {
		c.IP = ip4
	}
	if c.UdpPort == 0 {
		c.UdpPort = c.TcpPort
	}
	c.log.Infof("ip: %s tcp: %d", ipScheme, c.TcpPort)
	mAddr, err := ma.NewMultiaddr(fmt.Sprintf("/%s/%s/tcp/%d", ipScheme, c.IP.String(), c.TcpPort))
	if err != nil {
		return fmt.Errorf("could not construct multi addr: %v", err)
	}
	if err := h.Network().Listen(mAddr); err != nil {
		return err
	}
	c.Actor.IP = c.IP
	c.Actor.TcpPort = c.TcpPort
	c.Actor.UdpPort = c.UdpPort
	enr, err := addrutil.EnrToString(c.GetEnr())
	if err != nil {
		return err
	}
	c.log.WithField("enr", enr).Info("ENR")
	return nil
}

type HostViewCmd BasicCmd

func (c *HostViewCmd) Help() string {
	return "View local peer ID, listening addresses, etc."
}

func (c *HostViewCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	c.log.WithField("peer_id", h.ID()).Info("Peer ID")
	for _, a := range h.Addrs() {
		c.log.Infof("Listening on: %s", a.String())
	}
	enr, err := addrutil.EnrToString(c.GetEnr())
	if err != nil {
		return err
	}
	c.log.WithField("enr", enr).Info("ENR")
	return nil
}

type HostEventCmd BasicCmd

func (c *HostEventCmd) Help() string {
	return "Get notified of specific events, as long as the command runs."
}

func (c *HostEventCmd) HelpLong() string {
	return `
Args: <event-types>...

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
`
}

func (c *HostEventCmd) listenF(net network.Network, addr ma.Multiaddr) {
	c.log.WithFields(logrus.Fields{"event": "listen_open", "addr": addr.String()}).Debug("opened network listener")
}

func (c *HostEventCmd) listenCloseF(net network.Network, addr ma.Multiaddr) {
	c.log.WithFields(logrus.Fields{"event": "listen_close", "addr": addr.String()}).Debug("closed network listener")
}

func (c *HostEventCmd) connectedF(net network.Network, conn network.Conn) {
	c.log.WithFields(logrus.Fields{
		"event": "connection_open", "peer": conn.RemotePeer().String(),
		"direction": fmtDirection(conn.Stat().Direction),
	}).Debug("new peer connection")
}

func (c *HostEventCmd) disconnectedF(net network.Network, conn network.Conn) {
	c.log.WithFields(logrus.Fields{
		"event": "connection_close", "peer": conn.RemotePeer().String(),
		"direction": fmtDirection(conn.Stat().Direction),
	}).Debug("peer disconnected")
}

func (c *HostEventCmd) openedStreamF(net network.Network, str network.Stream) {
	c.log.WithFields(logrus.Fields{
		"event": "stream_open", "peer": str.Conn().RemotePeer().String(),
		"direction": fmtDirection(str.Stat().Direction),
		"protocol":  str.Protocol(),
	}).Debug("opened stream")
}

func (c *HostEventCmd) closedStreamF(net network.Network, str network.Stream) {
	c.log.WithFields(logrus.Fields{
		"event": "stream_close", "peer": str.Conn().RemotePeer().String(),
		"direction": fmtDirection(str.Stat().Direction),
		"protocol":  str.Protocol(),
	}).Debug("closed stream")
}

func (c *HostEventCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	bundle := &network.NotifyBundle{}
	for _, notifyType := range args {
		notifyType = strings.TrimSpace(notifyType)
		if notifyType == "" {
			continue
		}
		switch notifyType {
		case "listen_open":
			bundle.ListenF = c.listenF
		case "listen_close":
			bundle.ListenCloseF = c.listenCloseF
		case "connection_open":
			bundle.ConnectedF = c.connectedF
		case "connection_close":
			bundle.DisconnectedF = c.disconnectedF
		case "stream_open":
			bundle.OpenedStreamF = c.openedStreamF
		case "stream_close":
			bundle.ClosedStreamF = c.closedStreamF
		case "listen":
			bundle.ListenF = c.listenF
			bundle.ListenCloseF = c.listenCloseF
		case "connection":
			bundle.ConnectedF = c.connectedF
			bundle.DisconnectedF = c.disconnectedF
		case "stream":
			bundle.OpenedStreamF = c.openedStreamF
			bundle.ClosedStreamF = c.closedStreamF
		case "all":
			bundle.ListenF = c.listenF
			bundle.ListenCloseF = c.listenCloseF
			bundle.ConnectedF = c.connectedF
			bundle.DisconnectedF = c.disconnectedF
			bundle.OpenedStreamF = c.openedStreamF
			bundle.ClosedStreamF = c.closedStreamF
		default:
			return fmt.Errorf("unrecognized notification type: %s", notifyType)
		}
	}
	h.Network().Notify(bundle)
	<-ctx.Done()
	h.Network().StopNotify(bundle)
	return nil
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
