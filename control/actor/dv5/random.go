package dv5

import (
	"context"
	"github.com/ethereum/go-ethereum/log"
	"github.com/protolambda/rumor/control/actor/base"
)

type Dv5RandomCmd struct {
	*base.Base
	*Dv5State
}

func (c *Dv5RandomCmd) Help() string {
	return "Get random multi addrs, keep going until stopped"
}

func (c *Dv5RandomCmd) Run(ctx context.Context, args ...string) error {
	if c.Dv5State.Dv5Node == nil {
		return NoDv5Err
	}
	randomNodes := c.Dv5State.Dv5Node.RandomNodes()
	log.Info("Started looking for random nodes")

	go func() {
		<-ctx.Done()
		randomNodes.Close()
	}()
	for {
		if !randomNodes.Next() {
			break
		}
		res := randomNodes.Node()
		c.Log.WithField("node", res.String()).Infof("Got random node")
	}
	log.Info("Stopped looking for random nodes")
	return nil
}
