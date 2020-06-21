package dv5

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
)

type Dv5SelfCmd struct {
	*base.Base
	*Dv5State
}

func (c *Dv5SelfCmd) Help() string {
	return "get local discv5 ENR"
}

func (c *Dv5SelfCmd) Run(ctx context.Context, args ...string) error {
	if c.Dv5State.Dv5Node == nil {
		return NoDv5Err
	}
	c.Log.WithField("enr", c.Dv5State.Dv5Node.Self()).Infof("local dv5 node")
	return nil
}
