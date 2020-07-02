package dv5

import (
	"context"
	"fmt"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
)

type Dv5PingCmd struct {
	*base.Base
	*Dv5State
	Target flags.EnrOrEnodeFlag `ask:"<target>" help:"Target ENR/enode"`
}

func (c *Dv5PingCmd) Help() string {
	return "Run discv5-ping"
}

func (c *Dv5PingCmd) Run(ctx context.Context, args ...string) error {
	if c.Dv5State.Dv5Node == nil {
		return NoDv5Err
	}
	if err := c.Dv5State.Dv5Node.Ping(c.Target.Enode); err != nil {
		return fmt.Errorf("Failed to ping: %v", err)
	}
	c.Log.Infof("Successfully pinged")
	return nil
}
