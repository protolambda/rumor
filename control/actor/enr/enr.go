package enr

import (
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
)

type EnrCmd struct {
	*base.Base
}

func (c *EnrCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "view":
		cmd = &EnrViewCmd{Base: c.Base}
	case "gen-key":
		cmd = &EnrGenKeyCmd{Base: c.Base}
	case "make":
		cmd = &EnrMakeCmd{Base: c.Base}
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
