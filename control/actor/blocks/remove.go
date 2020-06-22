package blocks

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
)

type BlocksRemoveCmd struct {
	*base.Base
	BlockRoot beacon.Root `ask:"<root>" help:"Root of the block to delete"`
}

func (c *BlocksRemoveCmd) Help() string {
	return "Remove a block from the managed blocks collection"
}

func (c *BlocksRemoveCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
