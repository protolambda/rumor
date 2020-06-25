package head

import (
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
)

type HeadCmd struct {
	*base.Base
}

func (c *HeadCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "get":
		cmd = &GetCmd{Base: c.Base}
	case "set":
		cmd = &SetCmd{Base: c.Base}
	case "follow":
		cmd = &FollowCmd{Base: c.Base}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *HeadCmd) Routes() []string {
	return []string{"get", "set", "follow"}
}

func (c *HeadCmd) Help() string {
	return "Manage the head of the chain, either manually or with forkchoice"
}
