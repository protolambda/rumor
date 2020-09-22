package tool

import (
	"github.com/protolambda/ask"
	"io"
)

type ToolCmd struct {
	Out io.Writer
	// TODO: maybe log, and redirect log to std-err in case of running from CLI?
}

func (c *ToolCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "enr-value":
		cmd = &EnrValueGetCmd{Out: c.Out}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *ToolCmd) Routes() []string {
	return []string{"enr-value"}
}

func (c *ToolCmd) Help() string {
	return "Eth2 toolbelt, rumor commands without stateful context"
}
