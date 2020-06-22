package serve

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
)

type ByRangeCmd struct {
	*base.Base
	// TODO: compression option?
}

func (c *ByRangeCmd) Help() string {
	return "Serve the chain by slot range."
}

func (c *ByRangeCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
