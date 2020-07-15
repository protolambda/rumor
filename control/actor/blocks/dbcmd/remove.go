package dbcmd

import (
	"context"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
)

type BlocksRemoveCmd struct {
	*base.Base
	bdb.DB
	BlockRoot beacon.Root `ask:"<root>" help:"Root of the block to delete"`
}

func (c *BlocksRemoveCmd) Help() string {
	return "Remove a block from the managed blocks collection"
}

func (c *BlocksRemoveCmd) Run(ctx context.Context, args ...string) error {
	exists, err := c.DB.Remove(c.BlockRoot)
	if err != nil {
		return err
	}
	c.Log.WithField("existed", exists).Infof("removed block")
	return nil
}
