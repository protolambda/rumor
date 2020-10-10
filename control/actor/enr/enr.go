package enr

import (
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/peering/enrstate"
)

type LazyEnrState struct {
	Current *enrstate.EnrState
}

type EnrCmd struct {
	*base.Base
	Lazy *LazyEnrState
	base.PrivSettings
	base.WithHostPriv
}

func (c *EnrCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "view":
		cmd = &EnrViewCmd{Base: c.Base, Lazy: c.Lazy}
	case "gen-key":
		cmd = &EnrGenKeyCmd{Base: c.Base}
	case "make":
		cmd = &EnrMakeCmd{Base: c.Base, Lazy: c.Lazy, PrivSettings: c.PrivSettings, WithHostPriv: c.WithHostPriv}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *EnrCmd) Routes() []string {
	return []string{"view", "gen-key", "make"}
}

func (c *EnrCmd) Help() string {
	return "Ethereum Name Record (ENR) utilities"
}
