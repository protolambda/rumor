package chcmd

import (
	"context"
	"errors"
	"fmt"
	"github.com/protolambda/rumor/chain"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
)

type BlockCmd struct {
	*base.Base
	Blocks bdb.DB
	Chain  chain.FullChain
	Root   beacon.Root `ask:"<root>" help:"Root of the block to add to the chain"`
}

func (c *BlockCmd) Help() string {
	return "Add a block from the blocks tracker to the chain view."
}

func (c *BlockCmd) Run(ctx context.Context, args ...string) error {
	var block beacon.SignedBeaconBlock
	exists, err := c.Blocks.Get(c.Root, &block)
	if err != nil {
		return fmt.Errorf("could not get block for chain: %v", err)
	}
	if !exists {
		return errors.New("block does not exist")
	}
	if err := c.Chain.AddBlock(ctx, &block); err != nil {
		return fmt.Errorf("could not add block to chain: %v", err)
	}
	return nil
}
