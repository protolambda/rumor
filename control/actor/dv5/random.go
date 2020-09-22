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
	HandleENR `ask:"."`     // embed ENR handling command options
	Stepwise  bool          `ask:"--stepwise" help:"Step through each findnode action, instead of running automatically"`
	Interval  time.Duration `ask:"--interval" help:"Wait duration between findnode iterations, when not stepwise."`
}

func (c *Dv5RandomCmd) Default() {
	c.Add = true
	c.TTL = time.Hour * 24 * 30 * 3
	c.Stepwise = false
	c.Interval = 100 * time.Millisecond
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

	step := func() (exit bool, err error) {
		res := randomNodes.Node()
		if res == nil {
			return true, errors.New("no new node available")
		}
		entry := c.Log.WithFields(logrus.Fields{"enr": res.String(), "id": res.ID().String()})
		if err := c.HandleENR.handle(c.Log, res); err != nil {
			return false, err
		} else {
			entry.Infof("Got random node")
			return false, nil
		}
	}
	processingCtx, processCancel := context.WithCancel(context.Background())
	go func() {
		defer processCancel()
		for {
			if processingCtx.Err() != nil || !randomNodes.Next() {
				break
			}
			// If we are doing this step-wise, then wait for the controlling user to say "next"
			if c.Stepwise {
				if err := c.Control.Step(func(ctx context.Context) error {
					exit, err := step()
					if exit {
						processCancel()
					}
					return err
				}); err != nil {
					// not the same error as above func return. If stepping stopped, then err, and break immediately.
					break
				}
			} else {
				exit, err := step()
				if exit {
					processCancel()
					break
				}
				if err != nil {
					c.Log.WithError(err).Warn("dv5 findnode iteration failed")
				}
				// Wait for interval to complete, or command to end.
				ctx, _ := context.WithTimeout(processingCtx, c.Interval)
				<-ctx.Done()
			}
		}
	}()

	c.Control.RegisterStop(func(ctx context.Context) error {
		randomNodes.Close()
		processCancel()

		c.Log.Infof("Stopped looking for random nodes: %s", time.Now().String())
		return nil
	})
	return nil
}
