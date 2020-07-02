package dv5

import (
	"context"
	"fmt"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
)

type Dv5ResolveCmd struct {
	*base.Base
	*Dv5State
	Target flags.EnrOrEnodeFlag `ask:"<target>" help:"Target ENR/enode"`
}

func (c *Dv5ResolveCmd) Help() string {
	return "Resolve target address and try to find latest record for it."
}

func (c *Dv5ResolveCmd) Run(ctx context.Context, args ...string) error {
	if c.Dv5State.Dv5Node == nil {
		return NoDv5Err
	}
	resolved := c.Dv5State.Dv5Node.Resolve(c.Target.Enode)
	if resolved != nil {
		return fmt.Errorf("Failed to resolve %s, nil result", c.Target.String())
	}
	c.Log.WithField("enr", resolved.String()).Infof("Successfully resolved")
	return nil
}
