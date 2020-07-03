package control

import (
	"context"
	"fmt"
	"github.com/protolambda/rumor/control/actor"
	"github.com/sirupsen/logrus"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
	"strings"
	"sync"
	"sync/atomic"
)

type EnvGlobal interface {
	GetCall(id CallID) *Call
	MakeCall(actorID actor.ActorID, callID CallID, args []string) *Call
	IsClosing() bool
	KillActor(id actor.ActorID)
	GetJobs(id actor.ActorID) map[CallID]CallSummary
}

type SessionID string

type Session struct {
	global         EnvGlobal
	sessionID      SessionID
	lastCall       *Call
	interestsLock  sync.RWMutex
	interests      map[CallID]logrus.Level
	log            logrus.FieldLogger
	readNextScript func() (string, error)
	runner         *interp.Runner
	ctx            context.Context
	cancelSelf     context.CancelFunc

	callCounter   uint64
	requestedExit bool
}

func newSession(id SessionID, ctx context.Context, log logrus.FieldLogger, global EnvGlobal) *Session {
	ctx, cancel := context.WithCancel(ctx)
	sess := &Session{
		global:     global,
		sessionID:  id,
		lastCall:   nil,
		interests:  make(map[CallID]logrus.Level),
		log:        log,
		runner:     nil,
		ctx:        ctx,
		cancelSelf: cancel,
	}
	stdOutLog := log.WithField("dev", "out")
	stdErrLog := log.WithField("dev", "err")
	// ugly hack to map internal std-out and std-err to the log of the session, and to ignore std-in.
	// it ignores whitespace to avoid extra unnecessary log-entries.
	ioFns := interp.StdIO(nil,
		WriteableFn(func(msg string) {
			if strings.TrimSpace(msg) == "" {
				return
			}
			stdOutLog.Info(msg)
		}), WriteableFn(func(msg string) {
			if strings.TrimSpace(msg) == "" {
				return
			}
			stdErrLog.Error(msg)
		}))
	// never errors
	sess.runner, _ = interp.New(
		interp.ExecHandler(sess.RunCmd),
		interp.Env(sess),
		ioFns,
	)
	return sess
}

// Run interprets a node inside a Rumor session, using the mvdan shell package.
//
// From the shell docs:
// A node can be a *File, *Stmt, or Command. If a non-nil
// error is returned, it will typically contain commands exit status,
// which can be retrieved with IsExitStatus.
//
// Run can be called multiple times synchronously to interpret programs
// incrementally. To reuse a Runner without keeping the internal shell state,
// call Reset.
func (sess *Session) Run(ctx context.Context, node syntax.Node) error {
	err := sess.runner.Run(ctx, node)
	if sess.runner.Exited() {
		sess.log.Debug("Session finished")
		return sess.Close()
	}
	return err
}

func (sess *Session) Close() error {
	sess.requestedExit = true
	sess.cancelSelf()
	if sess.lastCall != nil {
		sess.log.Debug("Closing last call before exiting")
		sess.lastCall.Close()
		sess.lastCall = nil
	}
	return nil
}

func (sess *Session) Exited() bool {
	return sess.requestedExit
}

func (sess *Session) NewCallID() CallID {
	idNr := atomic.AddUint64(&sess.callCounter, 1)
	callID := CallID(fmt.Sprintf("%s_%d", sess.sessionID, idNr))
	return callID
}

func (sess *Session) GetLastCall() *Call {
	return sess.lastCall
}

func (sess *Session) SetInterest(id CallID, lvl logrus.Level) {
	sess.interestsLock.Lock()
	defer sess.interestsLock.Unlock()
	sess.interests[id] = lvl
}

func (sess *Session) UnsetInterest(id CallID) {
	sess.interestsLock.Lock()
	defer sess.interestsLock.Unlock()
	delete(sess.interests, id)
}

func (sess *Session) HasInterest(id CallID) (lvl logrus.Level, ok bool) {
	sess.interestsLock.RLock()
	defer sess.interestsLock.RUnlock()
	lvl, ok = sess.interests[id]
	return
}

func (sess *Session) ListInterests() (out []CallID) {
	sess.interestsLock.RLock()
	defer sess.interestsLock.RUnlock()
	for interest, _ := range sess.interests {
		out = append(out, interest)
	}
	return out
}

func (sess *Session) Done() <-chan struct{} {
	return sess.ctx.Done()
}

// RunCmd implements interp.ExecHandlerFunc
func (sess *Session) RunCmd(ctx context.Context, args []string) error {
	fmt.Printf("args: %s\n", strings.Join(args, " "))
	var actorName actor.ActorID
	var customCallID CallID
	{
		var actorStr, customCallIDStr, logLvlStr string
		skip := 0
		for _, arg := range args {
			if len(arg) == 0 {
				break
			}
			if strings.HasSuffix(arg, ":") && actorStr == "" {
				actorStr = arg[:len(arg)-1]
				skip++
			} else if strings.HasPrefix(arg, "_") && strings.HasSuffix(arg, "_") && customCallIDStr == "" {
				customCallIDStr = arg[1 : len(arg)-1]
				skip++
			} else if logLvlStr == "" && arg == "trace" { // TODO other log levels
				logLvlStr = arg
				skip++
			} else {
				break
			}
		}
		args = args[skip:]
		if actorStr == "" {
			actorStr = "DEFAULT_ACTOR"
		}
		if logLvlStr == "" {
			logLvlStr = "TRACE"
		}

		actorName = actor.ActorID(actorStr)
		customCallID = CallID(customCallIDStr)
		// TODO log level
	}

	// TODO option to cancel last call
	if len(args) == 1 && args[0] == "cancel" {
		sess.lastCall.Close()
		// TODO: could change exit code with special error return here.
		return nil
	}

	// TODO option to exit
	if len(args) == 1 && args[0] == "exit" {
		sess.requestedExit = true
		return nil
	}

	if len(args) == 1 && args[0] == "kill" {
		sess.log.Infof("Killing rumor p2p actor %s", actorName)
		sess.global.KillActor(actorName)
		return nil
	}

	if len(args) == 1 && args[0] == "jobs" {
		openJobs := sess.global.GetJobs(actorName)
		sess.log.WithField("jobs", openJobs).WithField("actor", actorName).Info("running jobs of actor")
	}

	callID := customCallID
	// auto-generate a call ID if we need to
	if callID == "" {
		callID = sess.NewCallID()
	}
	sess.SetInterest(callID, logrus.TraceLevel)

	// We remember the last call, even though we wait for it to be done,
	// since we may want to cancel/modify its spawned background processes, without having to specify the call ID again.
	sess.lastCall = sess.global.MakeCall(actorName, callID, args)
	<-sess.lastCall.doneCtx.Done()

	// TODO: could change exit code with special error return here.
	return nil
}

func (sess *Session) Get(name string) expand.Variable {
	// TODO
	value := name //transform key into value
	if value == "" {
		return expand.Variable{}
	}
	return expand.Variable{Exported: true, Kind: expand.String, Str: value}
}

func (sess *Session) Each(func(name string, vr expand.Variable) bool) {
	// no-op
}

var _ expand.Environ = (*Session)(nil)
