package head

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
)

type SetCmd struct {
	*base.Base
	Root beacon.Root `ask:"<root>" help:"The block to make the head. Must exist."`
}

func (c *SetCmd) Help() string {
	return "Override the head of the chain."
}

func (c *SetCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
