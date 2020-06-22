package blocks

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
)

type BlocksGetCmd struct {
	*base.Base
	BlockRoot beacon.Root `ask:"<root>" help:"Root of the block to get"`
}

func (c *BlocksGetCmd) Help() string {
	return "Get a summary of a SignedBeaconBlock by its block root"
}

func (c *BlocksGetCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
