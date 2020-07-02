package control

import (
	"context"
	"github.com/protolambda/rumor/control/actor"
	"github.com/sirupsen/logrus"
)

type CallID string

type CallSummary struct {
	ActorName actor.ActorID `json:"actor"`
	Args      []string      `json:"args"`
}

type CallExitReason uint8

const (
	SuccessDone CallExitReason = iota
	ParseError
	RuntimeError
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

// Close the call gracefully, blocking until it is freed
func (c *Call) Close() {
	// stop command, and wait gracefully for it to be done
	c.cancel()
	<-c.doneCtx.Done()
	// Stop background processes, and wait gracefully for them to be done
	c.closeSpawn()
	<-c.freeCtx.Done()
}
