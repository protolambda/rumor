package host

import (
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
)

type HostCmd struct {
	*base.Base
}

func (c *HostCmd) Help() string {
	return "Manage the libp2p host"
}

func (c *HostCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "start":
		cmd = DefaultHostStartCmd(c.Base)
	case "stop":
		cmd = &HostStopCmd{c.Base}
	case "view":
		cmd = &HostViewCmd{c.Base}
	case "listen":
		cmd = DefaultHostListenCmd(c.Base)
	case "event":
		cmd = &HostEventCmd{c.Base}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

