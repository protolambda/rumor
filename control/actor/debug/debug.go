package debug

import (
	"context"
	"github.com/protolambda/ask"
	"time"
)

type DebugCmd struct {
	*RootCmd
}

func (c *DebugCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "sleep":
		cmd = DebugSleepCmd{DebugCmd: c, Time: 1}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *DebugCmd) Help() string {
	return "For debugging purposes"
}

type DebugSleepCmd struct {
	*DebugCmd
	Time     time.Duration `ask:"<time>" help:"How long to sleep, e.g. 1s"`
}

func (c *DebugSleepCmd) Help() string {
	return "Sleep for given amount of milliseconds"
}

func (c *DebugSleepCmd) Run(ctx context.Context, args ...string) error {
	c.Log.Infoln("started sleeping for duration %s!", c.Time.String())
	sleepCtx, _ := context.WithTimeout(ctx, c.Time)
	<-sleepCtx.Done()
	if ctx.Err() == nil {
		c.Log.Infoln("done sleeping!")
	} else {
		c.Log.Infof("stopped sleep, exit early: %v", ctx.Err())
	}
	return nil
}
