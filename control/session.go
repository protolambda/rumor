package control

import (
	"context"
	"fmt"
	"github.com/protolambda/rumor/control/actor"
	"github.com/sirupsen/logrus"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type EnvGlobal interface {
	GetCall(id CallID) *Call
	MakeCall(actorID actor.ActorID, callID CallID, args []string) *Call
	IsClosing() bool
	KillActor(id actor.ActorID)
	GetCalls(id actor.ActorID) map[CallID]CallSummary
	GetLogData(key string) (value interface{}, ok bool)
	ClearLogData()
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
	parentEnv      expand.Environ

	defaultActorID actor.ActorID

	// if commands block, i.e. nicely wait for them to finish before freeing the runner for the next command.
	blocking bool

	callCounter   uint64
	requestedExit bool
}

func newSession(id SessionID, ctx context.Context, parentEnv expand.Environ, log logrus.FieldLogger, global EnvGlobal) *Session {
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
		parentEnv:  parentEnv,
		blocking:   true,
	}
	// ugly hack to map internal std-out and std-err to the log of the session, and to ignore std-in.
	// it ignores whitespace to avoid extra unnecessary log-entries.
	ioFns := interp.StdIO(nil,
		WriteableFn(func(msg string) {
			if strings.TrimSpace(msg) == "" {
				return
			}
			log.WithField("stdout", msg).Info("")
		}), WriteableFn(func(msg string) {
			if strings.TrimSpace(msg) == "" {
				return
			}
			log.WithField("stderr", msg).Error("")
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

func (sess *Session) SetBlocking(blocking bool) {
	sess.blocking = blocking
}

func (sess *Session) Close() error {
	sess.requestedExit = true
	sess.cancelSelf()
	if sess.lastCall != nil {
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
	actorName := sess.defaultActorID
	var customCallID CallID
	logLvl := logrus.DebugLevel
	{
		changedLogLvl := false
		var actorStr, customCallIDStr string
		skip := 0
		for _, arg := range args {
			if len(arg) == 0 {
				break
			}
			if strings.HasSuffix(arg, ":") && actorStr == "" {
				actorStr = arg[:len(arg)-1]
				skip++
			} else if strings.HasPrefix(arg, "_") && customCallIDStr == "" {
				customCallIDStr = arg[1:]
				skip++
			} else if !changedLogLvl && strings.HasPrefix(arg, "lvl_") {
				var err error
				logLvl, err = logrus.ParseLevel(arg[4:])
				if err != nil {
					break
				}
				changedLogLvl = true
				skip++
			} else {
				break
			}
		}
		args = args[skip:]
		if actorStr != "" {
			actorName = actor.ActorID(actorStr)
		}
		customCallID = CallID(customCallIDStr)
	}

	if len(args) == 1 && args[0] == "cancel" {
		sess.lastCall.Close()
		return sess.lastCall.exitReason.ExitErr()
	}

	if len(args) == 1 && args[0] == "kill" {
		sess.log.Infof("Killing rumor p2p actor %s", actorName)
		sess.global.KillActor(actorName)
		return nil
	}

	if len(args) == 1 && args[0] == "calls" {
		openJobs := sess.global.GetCalls(actorName)
		sess.log.WithField("calls", openJobs).WithField("actor", actorName).Info("list of running calls of actor")
		return nil
	}

	if len(args) == 1 && args[0] == "clear_log_data" {
		sess.global.ClearLogData()
		return nil
	}

	if len(args) == 1 && args[0] == "me" {
		sess.defaultActorID = actorName
		sess.log.Infof("Changed default actor to %s", actorName)
		return nil
	}

	if !actor.IsActorCmd(args) {
		return sess.RunNonRumorCmd(ctx, args)
	}

	callID := customCallID
	// auto-generate a call ID if we need to
	if callID == "" {
		callID = sess.NewCallID()
	}
	sess.SetInterest(callID, logLvl)

	// We remember the last call, even though we wait for it to be done,
	// since we may want to cancel/modify its spawned background processes, without having to specify the call ID again.
	sess.lastCall = sess.global.MakeCall(actorName, callID, args)
	if sess.blocking {
		<-sess.lastCall.doneCtx.Done()
	}

	return sess.lastCall.exitReason.ExitErr()
}

func (sess *Session) RunNonRumorCmd(ctx context.Context, args []string) error {
	// TODO: by modifying the handler-context in `ctx`
	// (get with interp.HandlerCtx(ctx)) things like std in/out and environment can be modified.

	// wait 5 seconds before killing a process after interrupting it (does not work on Windows)
	handler := interp.DefaultExecHandler(time.Second * 5)
	return handler(ctx, args)
}

func (sess *Session) Get(name string) expand.Variable {
	sess.log.WithField("envlookup", name).Trace("lookup!")
	if strings.HasPrefix(name, "_") {
		var val interface{}
		var ok bool
		if strings.HasPrefix(name, "__") {
			if last := sess.lastCall; last != nil {
				// still separated by a `_`, but first  `_` is a shortcut for the last call ID
				val, ok = sess.global.GetLogData(string(last.id) + name[1:])
			}
		} else {
			val, ok = sess.global.GetLogData(name[1:])
		}
		if ok {
			// if it's just a string, like most data, then avoid reflection and just return it.
			if vStr, ok := val.(string); ok {
				return expand.Variable{Exported: true, Kind: expand.String, Str: vStr}
			}
			if vMap, ok := val.(map[string]string); ok {
				return expand.Variable{Exported: true, Kind: expand.Associative, Map: vMap}
			}
			if vList, ok := val.([]string); ok {
				return expand.Variable{Exported: true, Kind: expand.Indexed, List: vList}
			}
			// Do some extra work with reflection for non-string types. To e.g. enable iteration over a list of peers.
			v := reflect.ValueOf(val)
			typ := v.Type()
			switch typ.Kind() {
			case reflect.Map:
				dat := make(map[string]string, v.Len())
				iter := v.MapRange()
				for iter.Next() {
					kStr := fmt.Sprintf("%v", iter.Key().Interface())
					dat[kStr] = fmt.Sprintf("%v", iter.Value().Interface())
				}
				return expand.Variable{Exported: true, Kind: expand.Associative, Map: dat}
			case reflect.Slice:
				dat := make([]string, v.Len(), v.Len())
				for i := 0; i < len(dat); i++ {
					dat[i] = fmt.Sprintf("%v", v.Index(i).Interface())
				}
				return expand.Variable{Exported: true, Kind: expand.Indexed, List: dat}
			default:
				return expand.Variable{Exported: true, Kind: expand.String, Str: fmt.Sprintf("%v", val)}
			}
		}
	}
	return sess.parentEnv.Get(name)
}

func (sess *Session) Each(fn func(name string, vr expand.Variable) bool) {
	// These are the variables that are passed to subshells (if not explicitly ignored).
	// It doesn't include anything dynamic from the regular session environment.
	// To run rumor inside rumor synchronously, with dynamic vars, use "include" instead of spawning a subshell.
	sess.parentEnv.Each(fn)
}

var _ expand.Environ = (*Session)(nil)
