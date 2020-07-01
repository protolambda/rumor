package serve

import (
	"context"
	"github.com/protolambda/rumor/chain"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	"github.com/protolambda/rumor/control/actor/base"
)

type ByRootCmd struct {
	*base.Base

	Chain chain.FullChain
	Blocks bdb.DB

	// TODO: option to enforce block to be part of canonical chain or not
}

func (c *ByRootCmd) Help() string {
	return "Serve the chain by block root."
}

func (c *ByRootCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
