package chain

import (
	"context"
	"github.com/protolambda/rumor/chain"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/sirupsen/logrus"
)

type ChainRemoveCmd struct {
	*base.Base
	chain.Chains
	Name chain.ChainID `ask:"<name>" help:"The name of the chain to remove. Must exist."`
}

func (c *ChainRemoveCmd) Help() string {
	return "Remove an eth2 chain, cannot remove when other actors are following the chain"
}

func (c *ChainRemoveCmd) Run(ctx context.Context, args ...string) error {
	existed := c.Chains.Remove(c.Name)
	c.Log.WithFields(logrus.Fields{"existed": existed, "name": c.Name}).Info("removed chain")
	return nil
}
