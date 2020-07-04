package chain

import (
	"context"
	"github.com/protolambda/rumor/chain"
	"github.com/protolambda/rumor/control/actor/base"
)

type ChainListCmd struct {
	*base.Base

	chain.Chains
	*ChainState
}

func (c *ChainListCmd) Help() string {
	return "List chains and identify current chain"
}

func (c *ChainListCmd) Run(ctx context.Context, args ...string) error {
	chainIDs := c.Chains.List()
	c.Log.WithField("chains", chainIDs).Infof("Got %d chains", len(chainIDs))
	if c.ChainState.CurrentChain != "" {
		c.Log.WithField("current", c.ChainState.CurrentChain).Info("Current chain")
	} else {
		c.Log.WithField("current", "").Info("No current chain")
	}
	return nil
}
