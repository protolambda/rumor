package dbcmd

import (
	"context"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	"github.com/protolambda/rumor/control/actor/base"
)

type BlocksListCmd struct {
	*base.Base
	bdb.DB
}

func (c *BlocksListCmd) Help() string {
	return "List known block roots"
}

func (c *BlocksListCmd) Run(ctx context.Context, args ...string) error {
	roots := c.DB.List()
	c.Log.WithField("block_roots", roots).Infof("got %d block roots", len(roots))
	return nil
}
