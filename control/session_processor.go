package control

import (
	"context"
	"fmt"
	"github.com/google/shlex"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor"
	"github.com/sirupsen/logrus"
	"strings"
	"sync"
	"time"
)

type LogSplitFn func(entry *logrus.Entry) error

func (fn LogSplitFn) Format(entry *logrus.Entry) ([]byte, error) {
	// we don't care about formatting the message contents, we just forward the entry itself elsewhere
	return nil, fn(entry)
}

type SessionID string

type Session struct {
	sessionID     SessionID
	interestsLock sync.RWMutex
	interests     map[CallID]logrus.Level
	log           logrus.FieldLogger
	readNextLine  func() (string, error)
	ctx           context.Context
}

func (s *Session) SetInterest(id CallID, lvl logrus.Level) {
	s.interestsLock.Lock()
	defer s.interestsLock.Unlock()
	s.interests[id] = lvl
}

func (s *Session) UnsetInterest(id CallID) {
	s.interestsLock.Lock()
	defer s.interestsLock.Unlock()
	delete(s.interests, id)
}

func (s *Session) HasInterest(id CallID) (lvl logrus.Level, ok bool) {
	s.interestsLock.RLock()
	defer s.interestsLock.RUnlock()
	lvl, ok = s.interests[id]
	return
}

func (s *Session) ListInterests() (out []CallID) {
	s.interestsLock.RLock()
	defer s.interestsLock.RUnlock()
	for interest, _ := range s.interests {
		out = append(out, interest)
	}
	return out
}

func (s *Session) Done() <-chan struct{} {
	return s.ctx.Done()
}

type SessionProcessor struct {
	adminLog logrus.FieldLogger

	// locks sessionIdCounter and sessions
	sessionsLock     sync.RWMutex
	sessions         map[*Session]struct{}
	sessionIdCounter uint64

	globalActorCtx    context.Context
	globalActorCancel context.CancelFunc

	// a map like map[string]*actor.Actor
	actors sync.Map

	// a map like map[CallID]*Call
	jobs      sync.Map
	log       logrus.FieldLogger
	closeLock sync.Mutex
	closing   bool
}

type VoidWriter struct{}

func (v VoidWriter) Write(p []byte) (n int, err error) {
	return len(p), err
}

func NewSessionProcessor(adminLog logrus.FieldLogger) *SessionProcessor {
	log := logrus.New()
	log.SetOutput(VoidWriter{})
	log.SetLevel(logrus.TraceLevel)

	globCtx, globCancel := context.WithCancel(context.Background())

	sp := &SessionProcessor{
		adminLog:          adminLog,
		sessions:          make(map[*Session]struct{}),
		log:               log,
		globalActorCtx:    globCtx,
		globalActorCancel: globCancel,
	}

	log.SetFormatter(LogSplitFn(func(entry *logrus.Entry) error {
		callIDi, ok := entry.Data["call_id"]
		if !ok {
			sp.adminLog.WithFields(entry.Data).Log(entry.Level, entry.Message)
			// Doesn't have a call-id, can be ignored.
			return nil
		}
		callID, ok := callIDi.(CallID)
		if !ok {
			callIDStr, ok := callIDi.(string)
			if !ok {
				return fmt.Errorf("cannot parse call id: %v", callIDi)
			}
			callID = CallID(callIDStr)
		}
		for s := range sp.sessions {
			if lvl, ok := s.HasInterest(callID); ok {
				// TODO: if this has lots of slow connection sessions open, we should parallelize and buffer this.
				if lvl >= entry.Level {
					s.log.WithFields(entry.Data).Log(entry.Level, entry.Message)
				}
			}
		}
		return nil
	}))

	return sp
}

type NextLineFn func() (string, error)

func (sp *SessionProcessor) NewSession(log logrus.FieldLogger, readNextLine NextLineFn) *Session {
	sp.sessionsLock.Lock()
	sp.sessionIdCounter += 1
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{
		sessionID:    SessionID(fmt.Sprintf("s%d", sp.sessionIdCounter)),
		interests:    make(map[CallID]logrus.Level),
		log:          log,
		readNextLine: readNextLine,
		ctx:          ctx,
	}
	sp.sessions[s] = struct{}{}
	sp.sessionsLock.Unlock()

	go func() {
		sp.runSession(s)
		// Declare the the session closed as soon as it exits
		sp.sessionsLock.Lock()
		delete(sp.sessions, s)
		sp.sessionsLock.Unlock()
		cancel()
	}()

	return s
}

type WriteableFn func(msg string)

func (fn WriteableFn) Write(p []byte) (n int, err error) {
	fn(string(p))
	return len(p), nil
}

type CallID string

type CallSummary struct {
	ActorName string   `json:"actor"`
	Args      []string `json:"args"`
}

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

	// Used to wait for resources to be completely free (including spawned processes)
	freeCtx context.Context
	// To indicate resources are freed
	free context.CancelFunc

	logger    logrus.FieldLogger
	actorName string
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

func (sp *SessionProcessor) GetActor(name string) *actor.Actor {
	a, _ := sp.actors.LoadOrStore(name, actor.NewActor(sp.globalActorCtx, actor.ActorID(name)))
	return a.(*actor.Actor)
}

func (sp *SessionProcessor) KillActor(name string) {
	// get actor
	a, ok := sp.actors.Load(name)
	if ok {
		// if there was an old one, close it
		a.(*actor.Actor).Close()
		sp.actors.Delete(name)
	}
}

func (sp *SessionProcessor) processCmd(actorName string, callID CallID, cmdArgs []string) *Call {
	rep := sp.GetActor(actorName)
	freeCtx, freeCancel := context.WithCancel(rep.ActorCtx)
	doneCtx, doneCancel := context.WithCancel(freeCtx)
	cmdCtx, cmdCancel := context.WithCancel(doneCtx)
	spawnCtx, spawnCancel := context.WithCancel(freeCtx)

	fmt.Printf("%s: %v\n", actorName, cmdArgs)

	cmdLogger := sp.log.WithField("actor", actorName).WithField("call_id", callID)

	call := &Call{
		id:         callID,
		args:       cmdArgs,
		ctx:        cmdCtx,
		cancel:     cmdCancel,
		doneCtx:    doneCtx,
		done:       doneCancel,
		spawnCtx:   spawnCtx,
		closeSpawn: spawnCancel,
		freeCtx:    freeCtx,
		free:       freeCancel,
		logger:     cmdLogger,
		actorName:  actorName,
	}

	callCmd := rep.MakeCmd(cmdLogger, call.Spawn)

	cmdLogger.Infof("Started! %s: %s", actorName, strings.Join(cmdArgs, " "))

	loadedCmd, err := ask.Load(callCmd)
	if err != nil {
		cmdLogger.WithError(err).Error("failed to parse command")
		freeCancel()
		sp.RemoveInterests(callID)
	} else {
		sp.jobs.Store(callID, call)
		go func() {
			cmdLogger.Tracef("Running! %s: ['%s']", actorName, strings.Join(cmdArgs, "', '"))
			fCmd, isHelp, err := loadedCmd.Execute(cmdCtx, cmdArgs...)
			if err != nil {
				cmdLogger.WithError(err).Error("exited with error")
			} else {
				if isHelp {
					cmdLogger.Info(fCmd.Usage())
				}
				cmdLogger.WithField("@success", "").Trace("completed call")
			}
			doneCancel()
			// If nothing was spawned, we can free the command early
			if !call.spawned {
				cmdLogger.Trace("Command is done, no background resources were spawned")
				freeCancel()
			} else {
				cmdLogger.Trace("Waiting for background tasks to be freed")
				// Wait for command to free
				<-freeCtx.Done()
			}
			cmdLogger.Trace("Finished, including optional spawned resources, removing call now")
			sp.RemoveInterests(callID)
			sp.jobs.Delete(callID)
		}()
	}

	return call
}

func (sp *SessionProcessor) RemoveInterests(id CallID) {
	sp.sessionsLock.RLock()
	for s := range sp.sessions {
		s.UnsetInterest(id)
	}
	sp.sessionsLock.RUnlock()
}

func (sp *SessionProcessor) Close() {
	sp.closeLock.Lock()
	defer sp.closeLock.Unlock()
	sp.closing = true

	var wg sync.WaitGroup
	sp.log.Debug("Closing remaining jobs...")
	// close remaining jobs
	sp.jobs.Range(func(ki, vi interface{}) bool {
		v := vi.(*Call)
		v.logger.Debug("Closing job with 5 second timeout...")
		v.cancel()
		wg.Add(1)
		closeCtx, _ := context.WithTimeout(v.freeCtx, time.Second*5)
		<-closeCtx.Done()
		if err := closeCtx.Err(); err != nil {
			v.logger.Error("Failed to close job with timeout")
		}
		wg.Done()
		return true
	})
	wg.Wait()

	sp.log.Debug("Closing remaining actors...")
	// close all libp2p hosts
	sp.actors.Range(func(key, value interface{}) bool {
		value.(*actor.Actor).Close()
		return true
	})

	sp.log.Debug("Closing global context...")
	// closes cross-actor things such as peerstores and chains
	sp.globalActorCancel()
	sp.log.Debug("Closed session processor")
}

func (sp *SessionProcessor) runSession(session *Session) {
	var lastCall *Call

	// count calls, for unique ID (if user does not specify their own ID for the call)
	callCounter := 0
	for {
		line, err := session.readNextLine()
		if err != nil {
			break
		}

		if sp.closing {
			session.log.Info("system is closing, cannot process more commands")
			break
		}

		line = strings.TrimSpace(line)
		// skip empty lines
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		// exits
		if line == "exit" {
			return
		}

		cmdArgs, err := shlex.Split(line)
		if err != nil {
			session.log.Errorf("Failed to parse command: %v\n", err)
			continue
		}
		if len(cmdArgs) == 0 {
			continue
		}

		if lastCall != nil && len(cmdArgs) == 1 {
			switch cmdArgs[0] {
			case "watch":
				session.log.Infof("Un-muting call '%s'", lastCall.id)
				session.SetInterest(lastCall.id, logrus.TraceLevel)
			case "mute":
				session.log.Infof("Muting call '%s'", lastCall.id)
				session.UnsetInterest(lastCall.id)
			case "bg":
				session.log.Infof("Moved call '%s' to background", lastCall.id)
				lastCall = nil
			case "cancel":
				session.log.Infof("Closing call '%s'", lastCall.id)
				lastCall.Close()
			default:
				break
			}
		}

		var actorName string
		var customCallID CallID
		{
			var actorStr, customCallIDStr, logLvlStr string
			skip := 0
			for _, arg := range cmdArgs {
				if len(arg) == 0 {
					break
				}
				if strings.HasSuffix(arg, ":") && actorStr == "" {
					actorStr = arg[:len(arg)-1]
					skip++
				} else if strings.HasSuffix(arg, ">") && customCallIDStr == "" {
					customCallIDStr = arg[:len(arg)-1]
					skip++
				} else if strings.HasSuffix(arg, "$") && logLvlStr == "" {
					logLvlStr = arg[:len(arg)-1]
					skip++
				} else {
					break
				}
			}
			cmdArgs = cmdArgs[skip:]
			if actorStr == "" {
				actorStr = "DEFAULT_ACTOR"
			}
			if logLvlStr == "" {
				logLvlStr = "TRACE"
			}

			actorName = actorStr
			customCallID = CallID(customCallIDStr)
			// TODO log level
		}

		if len(cmdArgs) == 0 {
			continue
		}

		if lastCall != nil {
			session.log.Debug("waiting for last call to complete")
			<-lastCall.doneCtx.Done()
		}

		if len(cmdArgs) == 1 && cmdArgs[0] == "kill" {
			session.log.Infof("Killing rumor p2p actor %s", actorName)
			sp.KillActor(actorName)
			continue
		}

		if len(cmdArgs) == 1 && cmdArgs[0] == "jobs" {
			openJobs := make(map[CallID]CallSummary, 0)
			sp.jobs.Range(func(key, value interface{}) bool {
				c := value.(*Call)
				if c.actorName == actorName {
					openJobs[key.(CallID)] = CallSummary{
						Args:      c.args,
						ActorName: c.actorName,
					}
				}
				return true
			})
			session.log.WithField("jobs", openJobs).WithField("actor", actorName).Info("running jobs of actor")
		}

		if customCallID != "" {
			// try historical call
			if c, ok := sp.jobs.Load(customCallID); ok {
				call := c.(*Call)
				if len(cmdArgs) == 1 {
					switch cmdArgs[0] {
					case "watch":
						session.log.Infof("Un-muting call '%s'", customCallID)
						session.SetInterest(customCallID, logrus.TraceLevel)
					case "mute":
						session.log.Infof("Muting call '%s'", customCallID)
						session.UnsetInterest(customCallID)
					case "fg":
						session.log.Infof("Moved call '%s' to foreground", customCallID)
						lastCall = call
					case "cancel":
						session.log.Infof("Closing call '%s'", customCallID)
						call.Close()
					}
				} else {
					session.log.Errorf("Unrecognized command for modifying call: '%s'", line)
				}
				continue
			}
		}

		background := false
		if cmdArgs[0] == "bg" {
			background = true
			cmdArgs = cmdArgs[1:]
		}

		if len(cmdArgs) == 0 {
			continue
		}

		callID := customCallID
		// auto-generate a call ID if we need to
		if callID == "" {
			callID = CallID(fmt.Sprintf("%s_%d", session.sessionID, callCounter))
			callCounter++
		}

		if len(cmdArgs) == 1 && cmdArgs[0] == "jobs" {
			continue
		}

		// TODO: option to change log level per command
		session.SetInterest(callID, logrus.TraceLevel)

		newCall := sp.processCmd(actorName, callID, cmdArgs)

		if !background {
			lastCall = newCall
		}
	}

	if lastCall != nil {
		session.log.Info("waiting for last call to complete")
		<-lastCall.doneCtx.Done()
	}
	session.log.Debug("Session finished")
}
