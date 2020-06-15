package debug

import (
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"time"
)

type DebugCmd struct {
	*base.Base
}

func (c *DebugCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "sleep":
		cmd = DebugSleepCmd{Base: c.Base, Time: time.Second}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *DebugCmd) Help() string {
	return "For debugging purposes"
}
