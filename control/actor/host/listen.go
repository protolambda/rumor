package host

import (
	"context"
	"errors"
	"fmt"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/protolambda/rumor/control/actor/base"
	"net"
)

type HostListenCmd struct {
	*base.Base
	base.WithEnrNode

	IP net.IP `ask:"--ip" help:"If no IP is specified, network interfaces are checked for one."`

	TcpPort uint16 `ask:"--tcp" help:"If no tcp port is specified, it defaults to the ENR TCP port."`
}

func (c *HostListenCmd) Default() {
	node, ok := c.WithEnrNode.GetNode()
	if !ok {
		// No ENR available yet? Just use the common eth2 ENR port
		c.TcpPort = 9000
	} else {
		c.TcpPort = uint16(node.TCP())
	}
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
		c.IP = net.IPv4zero
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
	mAddr, err := ma.NewMultiaddr(fmt.Sprintf("/%s/%s/tcp/%d", ipScheme, c.IP.String(), c.TcpPort))
	if err != nil {
		return fmt.Errorf("could not construct multi addr: %v", err)
	}
	if err := h.Network().Listen(mAddr); err != nil {
		return err
	}
	// Now with p2p, which is more useful to connect to as other node.
	mAddrWithp2p, err := ma.NewMultiaddr(fmt.Sprintf("/%s/%s/tcp/%d/p2p/%s",
		ipScheme, c.IP.String(), c.TcpPort, h.ID().String()))
	if err != nil {
		return fmt.Errorf("could not construct multi addr: %v", err)
	}
	c.Log.WithField("addr", mAddrWithp2p.String()).Infof("started listening on address")
	return nil
}
