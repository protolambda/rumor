package host

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
)

type HostStopCmd struct {
	*base.Base
	WithCloseHost
}

func (c *HostStopCmd) Help() string {
	return "Stop the host node."
}

func (c *HostStopCmd) Run(ctx context.Context, args ...string) error {
	return c.CloseHost()
}
