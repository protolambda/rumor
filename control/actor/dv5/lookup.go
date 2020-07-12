package dv5

import (
	"context"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
)

type Dv5LookupCmd struct {
	*base.Base
	*Dv5State
	Target flags.NodeIDFlexibleFlag `ask:"<target>" help:"Target ENR/enode/node-id"`
}

func (c *Dv5LookupCmd) Help() string {
	return "Get list of nearby nodes. If no target node is provided, then find nodes nearby to self."
}

func (c *Dv5LookupCmd) Run(ctx context.Context, args ...string) error {
	if c.Dv5State.Dv5Node == nil {
		return NoDv5Err
	}
	if c.Target.ID == (enode.ID{}) {
		c.Target.ID = c.Dv5State.Dv5Node.Self().ID()
	}

	res := c.Dv5State.Dv5Node.Lookup(c.Target.ID)
	enrs := make([]string, 0, len(res))
	for _, v := range res {
		enrs = append(enrs, v.String())
	}
	c.Log.WithField("nodes", enrs).Infof("Lookup complete")
	return nil
}
