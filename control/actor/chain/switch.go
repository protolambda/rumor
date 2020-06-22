package chain

import (
	"context"
	"github.com/protolambda/rumor/chain"
	"github.com/protolambda/rumor/control/actor/base"
)

type ChainSwitchCmd struct {
	*base.Base
	To chain.ChainID `ask:"<to>" help:"The name of the chain to switch to. Must exist."`
}

func (c *ChainSwitchCmd) Help() string {
	return "Switch actor to another eth2 chain"
}

func (c *ChainSwitchCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
