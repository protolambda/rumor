package host

import (
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"net"
)

type WithSetHost interface {
	SetHost(h host.Host) error
}

type WithCloseHost interface {
	CloseHost() error
}

type WithSetEnr interface {
	SetIP(ip net.IP)
	SetTCP(port uint16)
	SetUDP(port uint16)
}

type HostCmd struct {
	*base.Base
	WithSetHost
	WithSetEnr
	WithCloseHost
}

func (c *HostCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "start":
		cmd = &HostStartCmd{Base: c.Base, WithSetHost: c.WithSetHost}
	case "stop":
		cmd = &HostStopCmd{Base: c.Base, WithCloseHost: c.WithCloseHost}
	case "view":
		cmd = &HostViewCmd{c.Base}
	case "listen":
		cmd = &HostListenCmd{Base: c.Base, WithSetEnr: c.WithSetEnr}
	case "notify":
		cmd = &HostNotifyCmd{c.Base}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *HostCmd) Routes() []string {
	return []string{"start", "stop", "view", "listen", "event"}
}

func (c *HostCmd) Help() string {
	return "Manage the libp2p host"
}
