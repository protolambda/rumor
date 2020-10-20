package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/protolambda/rumor/control/actor"
	"github.com/sirupsen/logrus"
	"io"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type EnvGlobal interface {
	GetCall(id CallID) *Call
	// Runs the call in sync
	MakeCall(callCtx context.Context, out io.Writer, actorID actor.ActorID, callID CallID, args []string) (*Call, error)
	NewSubSession(log logrus.FieldLogger, ctx context.Context, parentEnv expand.Environ) *Session
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

	includesParser *syntax.Parser

	defaultActorID actor.ActorID

	callCounter   uint64
	requestedExit bool
}

func newSession(id SessionID, ctx context.Context, parentEnv expand.Environ, log logrus.FieldLogger, global EnvGlobal) *Session {
	ctx, cancel := context.WithCancel(ctx)
	sess := &Session{
		global:         global,
		sessionID:      id,
		lastCall:       nil,
		interests:      make(map[CallID]logrus.Level),
		log:            log,
		runner:         nil,
		ctx:            ctx,
		cancelSelf:     cancel,
		parentEnv:      parentEnv,
		defaultActorID: "DEFAULT_ACTOR_ID",
		includesParser: syntax.NewParser(),
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

func (sess *Session) Close() error {
	sess.requestedExit = true
	sess.cancelSelf()
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
	err := sess.runCmd(ctx, args)
	if err != nil {
		sess.log.WithError(err).WithField("args", args).Warn("failed")
	}
	return err
}

func (sess *Session) runCmd(ctx context.Context, args []string) error {
	actorName := sess.defaultActorID
	var customCallID CallID
	logLvl := logrus.DebugLevel
	changedLogLvl := false
	{
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

	if len(args) >= 1 && args[0] == "include" {
		if len(args) != 2 {
			sess.log.Errorf("Bad 'include' usage. Need a single script path, got %v.", args[1:])
			return ParseError.ExitErr()
		}
		handlingCtx := interp.HandlerCtx(ctx)
		// Temporarily change the default actor, to whatever the include was called with.
		// This makes running a snippet with a different default actor a one-liner: `bob: include some_task.rumor`
		prevActor := sess.defaultActorID
		sess.defaultActorID = actorName
		defer func() {
			sess.defaultActorID = prevActor
		}()
		scriptPath := args[1]
		if !path.IsAbs(scriptPath) {
			scriptPath = path.Join(handlingCtx.Dir, scriptPath)
		}
		return sess.RunInclude(ctx, scriptPath)
	}

	if len(args) >= 1 && args[0] == "next" {
		if len(args) > 2 {
			return fmt.Errorf("too many arguments, got %d, expected 2", len(args))
		}

		var call *Call
		if customCallID != "" {
			call = sess.global.GetCall(customCallID)
		} else {
			call = sess.lastCall
		}
		if call != nil {
			if len(args) == 2 {
				timeout, err := time.ParseDuration(args[1])
				if err != nil {
					return MaybeRuntimeErr(fmt.Errorf("step timeout could not be parsed: %v", err))
				}
				ctx, _ = context.WithTimeout(ctx, timeout)
			}

			noStep, finished, err := call.RequestStep(ctx)
			if err != nil {
				return MaybeRuntimeErr(fmt.Errorf("step failed: %v", err))
			}
			if noStep {
				return MaybeRuntimeErr(errors.New("no remaining steps"))
			}
			if finished {
				return MaybeRuntimeErr(errors.New("call finished, no more steps"))
			}
			return nil
		} else {
			return MaybeRuntimeErr(errors.New("call not available for stepping"))
		}
	}

	if len(args) >= 1 && args[0] == "cancel" {
		if len(args) > 2 {
			return MaybeRuntimeErr(fmt.Errorf("too many arguments, got %d, expected 2", len(args)))
		}
		var call *Call
		if customCallID != "" {
			call = sess.global.GetCall(customCallID)
		} else {
			call = sess.lastCall
		}
		if len(args) == 2 {
			timeout, err := time.ParseDuration(args[1])
			if err != nil {
				return MaybeRuntimeErr(fmt.Errorf("cancel timeout could not be parsed: %v", err))
			}
			ctx, _ = context.WithTimeout(ctx, timeout)
		}
		if call != nil {
			return MaybeRuntimeErr(func() error {
				if err := call.RequestStop(ctx); err != nil {
					return err
				}
				if sess.lastCall == call {
					sess.lastCall = nil
				}
				return nil
			}())
		} else {
			return MaybeRuntimeErr(errors.New("nothing to cancel"))
		}
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

	wOut := interp.HandlerCtx(ctx).Stdout

	// Useful hack to dump variables to Stdout in json format
	if len(args) >= 1 && args[0] == "grab" {
		if len(args) == 1 {
			return MaybeRuntimeErr(errors.New("specify the name of the var to grab"))
		}
		if len(args) != 2 {
			return MaybeRuntimeErr(errors.New("too many arguments"))
		}
		name := args[1]
		var val interface{}
		var ok bool
		if strings.HasPrefix(name, "__") {
			if last := sess.lastCall; last != nil {
				// still separated by a `_`, but first  `_` is a shortcut for the last call ID
				val, ok = sess.global.GetLogData(string(last.id) + name[1:])
			}
		} else if strings.HasPrefix(name, "_") {
			val, ok = sess.global.GetLogData(name[1:])
		} else {
			val, ok = sess.global.GetLogData(name)
			if !ok {
				if last := sess.lastCall; last != nil {
					val, ok = sess.global.GetLogData(string(last.id) + "_" + name)
				}
			}
		}
		if ok {
			enc := json.NewEncoder(wOut)
			return MaybeRuntimeErr(enc.Encode(val))
		} else {
			return MaybeRuntimeErr(errors.New("unknown variable"))
		}
	}

	if len(args) == 1 && args[0] == "me" {
		sess.defaultActorID = actorName
		sess.log.Infof("Changed default actor to %s", actorName)
		return nil
	}

	if len(args) == 0 {
		if changedLogLvl && customCallID != "" {
			sess.SetInterest(customCallID, logLvl)
			sess.log.Infof("Changed log level of %s to %s", customCallID, logLvl)
			return nil
		}
		return errors.New("no actual command to run")
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
	call, callErr := sess.global.MakeCall(ctx, wOut, actorName, callID, args)

	sess.lastCall = call

	return MaybeRuntimeErr(callErr)
}

// RunInclude runs an included file as a sub-session. The ctx must be an interp.HandlerCtx.
// The session environment, except the dynamic parts, is inherited. As well as the current default actor.
func (sess *Session) RunInclude(ctx context.Context, includePath string) error {
	handlingCtx := interp.HandlerCtx(ctx)
	subSession := sess.global.NewSubSession(sess.log.WithField("parent", sess.sessionID), ctx, handlingCtx.Env)
	subSession.defaultActorID = sess.defaultActorID
	defer subSession.Close()
	return subSession.runInclude(ctx, includePath)
}

func (sess *Session) runInclude(ctx context.Context, includePath string) error {
	inputFile, err := os.Open(includePath)
	if err != nil {
		sess.log.WithField("include", includePath).WithError(err).Error("failed to open included rumor script")
		return RuntimeError.ExitErr()
	}

	fileDoc, err := sess.includesParser.Parse(inputFile, includePath)
	_ = inputFile.Close()

	if err != nil {
		return ParseError.ExitErr()
	}

	subShell := sess.runner.Subshell()
	// make the script run with directory set to that of the script, to make code organization easier.
	subShell.Dir = filepath.Dir(includePath)
	sess.log.WithField("include", includePath).Info("Running included rumor script")
	return subShell.Run(ctx, fileDoc)
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
				return expand.Variable{Exported: true, Kind: expand.String, ReadOnly: true, Str: vStr}
			}
			if vMap, ok := val.(map[string]string); ok {
				return expand.Variable{Exported: true, Kind: expand.Associative, ReadOnly: true, Map: vMap}
			}
			if vList, ok := val.([]string); ok {
				return expand.Variable{Exported: true, Kind: expand.Indexed, ReadOnly: true, List: vList}
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
				return expand.Variable{Exported: true, Kind: expand.Associative, ReadOnly: true, Map: dat}
			case reflect.Slice:
				dat := make([]string, v.Len(), v.Len())
				for i := 0; i < len(dat); i++ {
					dat[i] = fmt.Sprintf("%v", v.Index(i).Interface())
				}
				return expand.Variable{Exported: true, Kind: expand.Indexed, ReadOnly: true, List: dat}
			default:
				return expand.Variable{Exported: true, Kind: expand.String, ReadOnly: true, Str: fmt.Sprintf("%v", val)}
			}
		}
	}
	if name == "ACTOR" {
		return expand.Variable{Exported: true, Kind: expand.String, ReadOnly: true, Str: string(sess.defaultActorID)}
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
