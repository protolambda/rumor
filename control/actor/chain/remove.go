package chain

import (
	"context"
	"github.com/protolambda/rumor/chain"
	"github.com/protolambda/rumor/control/actor/base"
)

type ChainRemoveCmd struct {
	*base.Base
	Name chain.ChainID `ask:"<name>" help:"The name of the chain to remove. Must exist."`
}

func (c *ChainRemoveCmd) Help() string {
	return "Remove an eth2 chain, cannot remove when other actors are following the chain"
}

func (c *ChainRemoveCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
