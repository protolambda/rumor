package dv5

import (
	"context"
	"errors"
	"github.com/ethereum/go-ethereum/log"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/sirupsen/logrus"
	"time"
)

type Dv5RandomCmd struct {
	*base.Base
	*Dv5State
	HandleENR // embed ENR handling command options
}

func (c *Dv5RandomCmd) Default() {
	c.Add = true
	c.TTL = time.Hour * 24 * 30 * 3
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

	h, err := c.Host()
	if err != nil {
		if c.Add {
			return errors.New("to add or connect nodes, a libp2p host needs to be started first")
		}
	}
	go func() {
		<-ctx.Done()
		randomNodes.Close()
	}()
	for {
		if !randomNodes.Next() {
			break
		}
		res := randomNodes.Node()
		c.HandleENR.handle(c.Log, h, res)
		c.Base.Log.WithFields(logrus.Fields{"enr": res.String(), "id": res.ID().String()}).Infof("Got random node")
	}
	log.Info("Stopped looking for random nodes")
	return nil
}
