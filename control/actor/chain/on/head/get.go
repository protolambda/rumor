package head

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
)

type GetCmd struct {
	*base.Base
}

func (c *GetCmd) Help() string {
	return "Get the head of the chain."
}

func (c *GetCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
