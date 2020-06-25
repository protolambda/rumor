package hot

import (
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
)

type HotCmd struct {
	*base.Base
}

func (c *HotCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "cold":
		cmd = &ViewCmd{Base: c.Base}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *HotCmd) Routes() []string {
	return []string{"view"}
}

func (c *HotCmd) Help() string {
	return "Manage the hot part of the chain"
}
