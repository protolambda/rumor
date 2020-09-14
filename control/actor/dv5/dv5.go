package dv5

import (
	"errors"

	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/peering/dv5"
	"github.com/protolambda/rumor/p2p/track"
)

type Dv5State struct {
	Dv5Node dv5.Discv5
}

type Dv5Cmd struct {
	*base.Base
	*Dv5State

	CurrentPeerstore track.DynamicPeerstore

	dv5.Dv5Settings
}

func (c *Dv5Cmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "run":
		cmd = &Dv5RunCmd{Base: c.Base, Dv5State: c.Dv5State, Dv5Settings: c.Dv5Settings}
	case "ping":
		cmd = &Dv5PingCmd{Base: c.Base, Dv5State: c.Dv5State}
	case "resolve":
		cmd = &Dv5ResolveCmd{Base: c.Base, Dv5State: c.Dv5State}
	case "request":
		cmd = &Dv5RequestCmd{Base: c.Base, Dv5State: c.Dv5State}
	case "lookup":
		cmd = &Dv5LookupCmd{Base: c.Base, Dv5State: c.Dv5State}
	case "random":
		cmd = &Dv5RandomCmd{Base: c.Base, Dv5State: c.Dv5State, HandleENR: HandleENR{Store: c.CurrentPeerstore}}
	case "self":
		cmd = &Dv5SelfCmd{Base: c.Base, Dv5State: c.Dv5State}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *Dv5Cmd) Routes() []string {
	return []string{"run", "ping", "resolve", "request", "lookup", "random", "self"}
}

func (c *Dv5Cmd) Help() string {
	return "Peer discovery with discv5"
}

var NoDv5Err = errors.New("Must start discv5 first. Try 'dv5 run'")
