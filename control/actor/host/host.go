package host

import (
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/track"
)

type WithSetHost interface {
	SetHost(h host.Host) error
}

type WithCloseHost interface {
	CloseHost() error
}

type HostCmd struct {
	*base.Base

	GlobalPeerstores track.Peerstores
	CurrentPeerstore track.DynamicPeerstore

	WithSetHost
	WithCloseHost
	base.PrivSettings
	base.WithEnrNode
}

func (c *HostCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "start":
		cmd = &HostStartCmd{Base: c.Base, WithSetHost: c.WithSetHost, PrivSettings: c.PrivSettings,
			GlobalPeerstores: c.GlobalPeerstores, CurrentPeerstore: c.CurrentPeerstore}
	case "stop":
		cmd = &HostStopCmd{Base: c.Base, WithCloseHost: c.WithCloseHost}
	case "view":
		cmd = &HostViewCmd{Base: c.Base, WithEnrNode: c.WithEnrNode}
	case "listen":
		cmd = &HostListenCmd{Base: c.Base, WithEnrNode: c.WithEnrNode}
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
