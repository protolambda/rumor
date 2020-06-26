package chain

import (
	"context"
	"errors"
	"github.com/protolambda/rumor/chain"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/sirupsen/logrus"
)

type ChainSwitchCmd struct {
	*base.Base
	*ChainState
	To chain.ChainID `ask:"<to>" help:"The name of the chain to switch to. Must exist."`
}

func (c *ChainSwitchCmd) Help() string {
	return "Switch actor to another eth2 chain"
}

func (c *ChainSwitchCmd) Run(ctx context.Context, args ...string) error {
	prev := c.ChainState.CurrentChain
	if c.To == "" {
		return errors.New("need a chain name to switch to")
	}
	c.ChainState.CurrentChain = c.To
	c.Log.WithFields(logrus.Fields{"from": prev, "to": c.To}).Info("switched chains")
	return nil
}
