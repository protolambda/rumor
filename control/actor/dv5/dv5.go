package dv5

import (
	"errors"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/peering/dv5"
)

type Dv5State struct {
	Dv5Node dv5.Discv5
}

type Dv5Cmd struct {
	*base.Base
}

func (c *Dv5Cmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "start":
		cmd = &Dv5StartCmd{Base: c.Base}
	case "stop":
		cmd = &Dv5StopCmd{Base: c.Base}
	case "ping":
		cmd = &Dv5PingCmd{Base: c.Base}
	case "resolve":
		cmd = &Dv5ResolveCmd{Base: c.Base}
	case "request":
		cmd = &Dv5RequestCmd{Base: c.Base}
	case "lookup":
		cmd = &Dv5LookupCmd{Base: c.Base}
	case "random":
		cmd = &Dv5RandomCmd{Base: c.Base}
	case "self":
		cmd = &Dv5SelfCmd{Base: c.Base}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *Dv5Cmd) Routes() []string {
	return []string{"start", "stop", "ping", "resolve", "request", "lookup", "random", "self"}
}

func (c *Dv5Cmd) Help() string {
	return "Peer discovery with discv5"
}

var NoDv5Err = errors.New("Must start discv5 first. Try 'dv5 start'")

