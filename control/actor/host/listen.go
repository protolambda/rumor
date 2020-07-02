package host

import (
	"context"
	"errors"
	"fmt"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/addrutil"
	"net"
)

type HostListenCmd struct {
	*base.Base
	WithSetEnr

	IP net.IP `ask:"--ip" help:"If no IP is specified, network interfaces are checked for one."`

	TcpPort uint16 `ask:"--tcp" help:"If no tcp port is specified, it defaults to 9000."`
	UdpPort uint16 `ask:"--udp" help:"If no udp port is specified (= 0), UDP equals TCP."`
}

func (c *HostListenCmd) Default() {
	c.TcpPort = 9000
	c.UdpPort = 0
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
	c.Log.Infof("ip: %s tcp: %d", ipScheme, c.TcpPort)
	mAddr, err := ma.NewMultiaddr(fmt.Sprintf("/%s/%s/tcp/%d", ipScheme, c.IP.String(), c.TcpPort))
	if err != nil {
		return fmt.Errorf("could not construct multi addr: %v", err)
	}
	if err := h.Network().Listen(mAddr); err != nil {
		return err
	}
	c.WithSetEnr.SetIP(c.IP)
	c.WithSetEnr.SetTCP(c.TcpPort)
	c.WithSetEnr.SetUDP(c.UdpPort)
	enr, err := addrutil.EnrToString(c.GetEnr())
	if err != nil {
		return err
	}
	c.Log.WithField("enr", enr).Info("ENR")
	return nil
}
