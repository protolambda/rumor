package debug

import (
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
)

type DebugCmd struct {
	*base.Base
}

func (c *DebugCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "sleep":
		cmd = &DebugSleepCmd{Base: c.Base}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *DebugCmd) Routes() []string {
	return []string{"sleep"}
}

func (c *DebugCmd) Help() string {
	return "For debugging purposes"
}
