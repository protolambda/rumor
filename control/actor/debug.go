package actor

import (
	"context"
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"time"
)

type DebugCmd struct {
	*Actor `ask:"-"`
	log    logrus.FieldLogger
}

func (c *DebugCmd) Get(ctx context.Context, args ...string) (cmd interface{}, remaining []string, err error) {
	if len(args) == 0 {
		return nil, nil, errors.New("no subcommand specified")
	}
	switch args[0] {
	case "sleep":
		cmd = &DebugSleepCmd{
			Actor: c.Actor,
			log:   c.log,
		}
	default:
		return nil, args, fmt.Errorf("unrecognized command: %v", args)
	}
	return cmd, args[1:], nil
}

func (c *DebugCmd) Help() string {
	return "For debugging purposes" // TODO list subcommands
}

type DebugSleepCmd struct {
	*Actor `ask:"-"`
	log    logrus.FieldLogger
	Ms     uint64 `ask:"<ms>" help:"How long to sleep, in milliseconds"`
}

func (c *DebugSleepCmd) Help() string {
	return "Sleep for given amount of milliseconds"
}

func (c *DebugSleepCmd) Run(ctx context.Context, args ...string) error {
	c.log.Infoln("started sleeping!")
	sleepCtx, _ := context.WithTimeout(ctx, time.Duration(c.Ms)*time.Millisecond)
	<-sleepCtx.Done()
	if ctx.Err() == nil {
		c.log.Infoln("done sleeping!")
	} else {
		c.log.Infof("stopped sleep, exit early: %v", ctx.Err())
	}
	return nil
}
