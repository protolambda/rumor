package dv5

import (
	"errors"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/peering/dv5"
	"github.com/protolambda/rumor/p2p/track"
)

type WithPriv interface {
	GetPriv() crypto.PrivKey
}

type Dv5State struct {
	Dv5Node dv5.Discv5
}

type Dv5Cmd struct {
	*base.Base
	*Dv5State

	PeerInfoFinder track.PeerInfoFinder

	WithPriv
}

func (c *Dv5Cmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "start":
		cmd = &Dv5StartCmd{Base: c.Base, Dv5State: c.Dv5State, WithPriv: c.WithPriv}
	case "stop":
		cmd = &Dv5StopCmd{Base: c.Base, Dv5State: c.Dv5State}
	case "ping":
		cmd = &Dv5PingCmd{Base: c.Base, Dv5State: c.Dv5State}
	case "resolve":
		cmd = &Dv5ResolveCmd{Base: c.Base, Dv5State: c.Dv5State}
	case "request":
		cmd = &Dv5RequestCmd{Base: c.Base, Dv5State: c.Dv5State}
	case "lookup":
		cmd = &Dv5LookupCmd{Base: c.Base, Dv5State: c.Dv5State}
	case "random":
		cmd = &Dv5RandomCmd{Base: c.Base, Dv5State: c.Dv5State,
			HandleENR: HandleENR{
				PeerInfoFinder: c.PeerInfoFinder,
			}}
	case "self":
		cmd = &Dv5SelfCmd{Base: c.Base, Dv5State: c.Dv5State}
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
