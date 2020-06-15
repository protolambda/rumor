package dv5

import (
	"github.com/ethereum/go-ethereum/log"
	"github.com/protolambda/rumor/control/actor/base"
)

type Dv5StopCmd struct {
	*base.Base
}

func (c *Dv5StopCmd) Help() string {
	return "Stop discv5"
}

func (c *Dv5StopCmd) Run(ctx context.Context, args ...string) error {
	if c.Dv5State.Dv5Node == nil {
		return NoDv5Err
	}
	c.Dv5State.Dv5Node.Close()
	c.Dv5State.Dv5Node = nil
	log.Info("Stopped discv5")
	return nil
}

