package debug

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"time"
)

type DebugSleepCmd struct {
	*base.Base
	Time time.Duration `ask:"<time>" help:"How long to sleep, e.g. 1s"`
}

func (c *DebugSleepCmd) Help() string {
	return "Sleep for given amount of milliseconds"
}

func (c *DebugSleepCmd) Default() {
	c.Time = time.Second
}

func (c *DebugSleepCmd) Run(ctx context.Context, args ...string) error {
	if c.Time != 0 {
		c.Log.Infof("started sleeping for duration %s!", c.Time.String())
		ticker := time.NewTicker(c.Time)
		select {
		case <-ticker.C:
			c.Log.Infoln("done sleeping!")
			break
		case <-ctx.Done():
			c.Log.Infof("stopped sleep, exit early: %v", ctx.Err())
			break
		}
		ticker.Stop()
	} else {
		c.Log.Warn("cannot sleep for 0 duration")
	}
	return nil
}
