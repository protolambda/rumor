package dv5

import (
	"context"
	"errors"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/sirupsen/logrus"
	"time"
)

type Dv5RandomCmd struct {
	*base.Base
	*Dv5State
	HandleENR `ask:"."` // embed ENR handling command options
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
	c.Log.Infof("Started looking for random nodes: %s", time.Now().String())

	go func() {
		exit := false
		for {
			if exit || !randomNodes.Next() {
				break
			}
			if err := c.Control.Step(func(ctx context.Context) error {
				res := randomNodes.Node()
				if res == nil {
					exit = true
					return errors.New("no new node available")
				}
				entry := c.Log.WithFields(logrus.Fields{"enr": res.String(), "id": res.ID().String()})
				if err := c.HandleENR.handle(c.Log, res); err != nil {
					return err
				} else {
					entry.Infof("Got random node")
					return nil
				}
			}); err != nil {
				// no more steps, stop looping.
				break
			}
		}
	}()

	c.Control.RegisterStop(func(ctx context.Context) error {
		randomNodes.Close()

		c.Log.Infof("Stopped looking for random nodes: %s", time.Now().String())
		return nil
	})
	return nil
}
