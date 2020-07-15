package cold

import (
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
)

type ColdCmd struct {
	*base.Base
}

func (c *ColdCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "cold":
		cmd = &ViewCmd{Base: c.Base}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *ColdCmd) Routes() []string {
	return []string{"view"}
}

func (c *ColdCmd) Help() string {
	return "Manage the cold part of the chain"
}
