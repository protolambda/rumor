package dv5

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
)

type Dv5RequestCmd struct {
	*base.Base
	*Dv5State
	Target flags.EnrOrEnodeFlag `ask:"<target>" help:"Target ENR/enode"`
}

func (c *Dv5RequestCmd) Help() string {
	return "Request target address directly."
}

func (c *Dv5RequestCmd) Run(ctx context.Context, args ...string) error {
	if c.Dv5State.Dv5Node == nil {
		return NoDv5Err
	}
	enrRes, err := c.Dv5State.Dv5Node.RequestENR(c.Target.Enode)
	if err != nil {
		return err
	}
	c.Log.WithField("enr", enrRes.String()).Infof("Successfully got ENR for node")
	return nil
}
