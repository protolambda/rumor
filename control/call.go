package control

import (
	"context"
	"errors"
	"fmt"
	"github.com/protolambda/rumor/control/actor"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/sirupsen/logrus"
	"mvdan.cc/sh/v3/interp"
	"sync"
)

type CallID string

type CallSummary struct {
	ActorName actor.ActorID `json:"actor"`
	Args      []string      `json:"args"`
}

type CallErr struct {
	Exit CallExitReason
	Err  error
}

func (ce *CallErr) String() string {
	return ce.Error()
}

func (ce *CallErr) Error() string {
	return fmt.Sprintf("%s: %s", ce.Exit.ExitErr().Error(), ce.Err.Error())
}

func (ce *CallErr) Unwrap() error {
	return ce.Exit.ExitErr()
}

func MaybeRuntimeErr(err error) error {
	if err != nil {
		return RuntimeError.WithErr(err)
	} else {
		return nil
	}
}

type CallExitReason uint8

func (code CallExitReason) ExitErr() error {
	return interp.NewExitStatus(uint8(code))
}

func (code CallExitReason) WithErr(err error) *CallErr {
	return &CallErr{Exit: code, Err: err}
}

const (
	SuccessDone CallExitReason = iota
	RuntimeError
	ParseError
)

type Call struct {
	id          CallID
	args        []string
	startTimeNS int64

	onStopLock sync.Mutex
	onStop     base.OnStop
	steps      chan base.Step

	spawned bool

	// Used to wait for resources to be completely bgCancel (including spawned processes)
	bgCtx context.Context
	// To indicate resources are freed
	bgCancel context.CancelFunc

	logger    logrus.FieldLogger
	actorName actor.ActorID
}

func (c *Call) RegisterStop(onStop base.OnStop) {
	c.onStopLock.Lock()
	defer c.onStopLock.Unlock()
	if c.onStop != nil {
		prev := c.onStop
		c.onStop = func(ctx context.Context) error {
			// Try to stop both tasks
			err1 := prev(ctx)
			err2 := onStop(ctx)
			if err1 != nil || err2 != nil {
				return fmt.Errorf("err1: %v err2: %v", err1, err2)
			}
			return nil
		}
	} else {
		c.onStop = onStop
	}
	c.spawned = true
}

// Blocks until the step is consumed by the caller
func (c *Call) Step(step base.Step) error {
	select {
	case c.steps <- step:
		return nil
	case <-c.bgCtx.Done():
		return errors.New("call is over, no more steps")
	}
}

// RequestStep wait for the next step, and waits for the step to complete, or for the call to finish.
func (c *Call) RequestStep(ctx context.Context) (noStep bool, finish bool, err error) {
	select {
	case <-ctx.Done():
		return false, false, errors.New("step request stopped")
	case step, ok := <-c.steps:
		err = step(ctx)
		c.logger.WithField("__step", true).Trace("Step complete")
		return !ok, false, err
	case <-c.bgCtx.Done():
		return true, true, nil
	}
}

func (c *Call) RequestStop(ctx context.Context) error {
	c.onStopLock.Lock()
	defer c.onStopLock.Unlock()
	if c.onStop != nil {
		err := c.onStop(ctx)
		if err == nil {
			// only clean up the stop function if it did not error.
			c.onStop = nil
		}
		if err != nil {
			return err
		}
	}
	c.bgCancel()
	// wait for timeout, or proper cleanup
	select {
	case <-c.bgCtx.Done():
		break
	case <-ctx.Done():
		break
	}
	return nil
}
