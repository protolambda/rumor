package on

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
)

type BlockCmd struct {
	*base.Base
	Root beacon.Root `ask:"<root>" help:"Root of the block to add to the chain"`
}

func (c *BlockCmd) Help() string {
	return "Add a block from the blocks tracker to the chain view."
}

func (c *BlockCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
