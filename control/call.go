package control

import (
	"context"
	"github.com/protolambda/rumor/control/actor"
	"github.com/sirupsen/logrus"
	"mvdan.cc/sh/v3/interp"
	"sync"
)

type CallID string

type CallSummary struct {
	ActorName actor.ActorID `json:"actor"`
	Args      []string      `json:"args"`
}

type CallExitReason uint8

func (code CallExitReason) ExitErr() error {
	return interp.NewExitStatus(uint8(code))
}

const (
	SuccessDone CallExitReason = iota
	RuntimeError
	ParseError
)

type Call struct {
	id   CallID
	args []string

	// context for immediate resources of the command
	ctx    context.Context
	cancel context.CancelFunc
	// context that completes after the command finishes running (excludes spawned processes)
	doneCtx context.Context
	done    context.CancelFunc

	stepLock      sync.Mutex
	nextComplete  context.Context
	nextCompleter context.CancelFunc
	nextRequest   context.Context
	nextRequester context.CancelFunc

	// context for spawned background processes
	spawnCtx   context.Context
	closeSpawn context.CancelFunc

	spawned bool

	// Only applies to the immediate running, not later background tasks
	exitReason CallExitReason

	// Used to wait for resources to be completely free (including spawned processes)
	freeCtx context.Context
	// To indicate resources are freed
	free context.CancelFunc

	logger    logrus.FieldLogger
	actorName actor.ActorID
}

func (c *Call) Spawn() (ctx context.Context, done context.CancelFunc) {
	c.spawned = true
	return c.spawnCtx, c.free
}

// Step blocks if there is already a step in progress.
func (c *Call) Step() (ctx context.Context, done context.CancelFunc) {
	c.stepLock.Lock()
	defer c.stepLock.Unlock()
	// complete previous step, if any.
	if c.nextComplete != nil {
		c.nextCompleter()
	}
	c.nextComplete, c.nextCompleter = context.WithCancel(c.freeCtx)
	c.nextRequest, c.nextRequester = context.WithCancel(c.nextComplete)
	return c.nextRequest, c.nextCompleter
}

// RequestStep steps into any open step, and waits for the step to complete.
func (c *Call) RequestStep() (noStep bool) {
	c.stepLock.Lock()
	nextFn := c.nextRequester
	compl := c.nextComplete
	c.stepLock.Unlock()
	if nextFn == nil {
		return true
	}
	nextFn()
	<-compl.Done()
	return false
}

// Close the call gracefully, blocking until it is freed
func (c *Call) Close() {
	if c == nil {
		return
	}
	// stop command, and wait gracefully for it to be done
	c.cancel()
	<-c.doneCtx.Done()
	// Stop background processes, and wait gracefully for them to be done
	c.closeSpawn()
	<-c.freeCtx.Done()
}
