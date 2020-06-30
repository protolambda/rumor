package sync

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
)

type ByRootCmd struct {
	*base.Base
	// TODO: compression option?
}

func (c *ByRootCmd) Help() string {
	return "Sync the chain by block root."
}

func (c *ByRootCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
