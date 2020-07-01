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

	sp := &SessionProcessor{
		adminLog: adminLog,
		sessions: make(map[*Session]struct{}),
		log:      log,
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
		cancel()
		sp.sessionsLock.Lock()
		delete(sp.sessions, s)
		sp.sessionsLock.Unlock()
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
	Owner     CallOwner `json:"owner"`
	ActorName string    `json:"actor"`
}

type CallOwner string

type Call struct {
	ctx       context.Context
	cancel    context.CancelFunc
	logger    logrus.FieldLogger
	owner     CallOwner
	actorName string
}

func (sp *SessionProcessor) GetActor(name string) *actor.Actor {
	a, _ := sp.actors.LoadOrStore(name, actor.NewActor(actor.ActorID(name)))
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

func (sp *SessionProcessor) processCmd(actorName string, callID CallID, owner CallOwner, cmdArgs []string) {
	rep := sp.GetActor(actorName)
	cmdCtx, cmdCancel := context.WithCancel(rep.ActorCtx)

	cmdLogger := sp.log.WithField("actor", actorName).WithField("call_id", callID)

	call := &Call{
		ctx:       cmdCtx,
		cancel:    cmdCancel,
		logger:    cmdLogger,
		owner:     owner,
		actorName: actorName,
	}

	sp.jobs.Store(callID, call)

	go func() {
		callCmd := rep.MakeCmd(cmdLogger)

		loadedCmd, err := ask.Load(callCmd)
		if err != nil {
			cmdLogger.WithError(err).Error("failed to parse command")
		} else {
			fCmd, isHelp, err := loadedCmd.Execute(cmdCtx, cmdArgs...)
			if err != nil {
				cmdLogger.WithError(err).Error("exited with error")
			} else {
				if isHelp {
					cmdLogger.Info(fCmd.Usage())
				}
				cmdLogger.WithField("@success", "").Trace("completed call")
			}
		}
		sp.CloseCall(callID)
		// Only remove interests when the call is done, not when the call is closed,
		// at which point it is still running and the `@success` hasn't been posted yet.
		sp.RemoveInterests(callID)
	}()
}

func (sp *SessionProcessor) CloseCall(id CallID) {
	ci, ok := sp.jobs.Load(id)
	if !ok {
		return
	}
	c := ci.(*Call)
	// safe to cancel multiple times, safe to unset interest multiple times.
	// No global locking at the cost of a little overhead sometimes.
	// TODO: once we have Go 1.15 we can use LoadAndDelete, see golang #33762
	c.cancel()
	sp.jobs.Delete(id)
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
	// close remaining jobs
	sp.jobs.Range(func(ki, vi interface{}) bool {
		k := ki.(CallID)
		v := vi.(*Call)
		v.logger.Info("Closing job with 5 second timeout...")
		v.cancel()
		wg.Add(1)
		go func(k CallID, ctx context.Context, log logrus.FieldLogger) {
			ctx, _ = context.WithTimeout(ctx, time.Second*5)
			<-ctx.Done()
			if err := ctx.Err(); err != nil {
				if err != context.Canceled {
					log.Error(err)
				}
			}
			wg.Done()
		}(k, v.ctx, v.logger)
		return true
	})
	wg.Wait()

	// close all libp2p hosts
	sp.actors.Range(func(key, value interface{}) bool {
		value.(*actor.Actor).Close()
		return true
	})
}

func (sp *SessionProcessor) runSession(session *Session) {
	var lastCall CallID

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

		var actorName string
		var customCallID CallID
		var owner CallOwner
		{
			var actorStr, customCallIDStr, ownerStr string
			skip := 0
			for _, arg := range cmdArgs {
				if len(arg) <= 1 {
					break
				}
				if strings.HasSuffix(arg, ":") && actorStr == "" {
					actorStr = arg[:len(arg)-1]
					skip++
				} else if strings.HasSuffix(arg, ">") && customCallIDStr == "" {
					customCallIDStr = arg[:len(arg)-1]
					skip++
				} else if strings.HasSuffix(arg, "$") && ownerStr == "" {
					ownerStr = arg[:len(arg)-1]
					skip++
				} else {
					break
				}
			}
			cmdArgs = cmdArgs[skip:]
			if actorStr == "" {
				actorStr = "DEFAULT_ACTOR"
			}
			if ownerStr == "" {
				ownerStr = "DEFAULT_OWNER"
			}

			actorName = actorStr
			customCallID = CallID(customCallIDStr)
			owner = CallOwner(ownerStr)
		}

		if len(cmdArgs) == 0 {
			continue
		}

		if len(cmdArgs) == 1 && cmdArgs[0] == "kill" {
			session.log.Infof("Killing rumor p2p actor %s", actorName)
			sp.KillActor(actorName)
			continue
		}

		if lastCall != "" && (customCallID == "" || customCallID == lastCall) {
			c, ok := sp.jobs.Load(lastCall)
			if ok {
				lastCallObj := c.(*Call)
				if existingOwner := lastCallObj.owner; existingOwner != owner {
					session.log.Errorf("No access to call of %s", existingOwner)
					continue
				} else {
					if len(cmdArgs) == 1 && cmdArgs[0] == "cancel" {
						session.log.Infof("Closing call '%s'", lastCall)
						sp.CloseCall(lastCall)
						continue
					} else if len(cmdArgs) == 1 && cmdArgs[0] == "bg" {
						session.log.Infof("Moved call '%s' to background", lastCall)
						lastCall = ""
						continue
					} else if customCallID == "" {
						// if a next command and we are here by accident, wait for current call to stop
						<-lastCallObj.ctx.Done()
						lastCall = ""
					}
				}
			} else {
				// call does not exist anymore, it ended. Safe to do another call with new id.
				lastCall = ""
			}
		}

		if customCallID != "" {
			// try historical call
			if c, ok := sp.jobs.Load(customCallID); ok {
				if existingOwner := c.(*Call).owner; existingOwner != owner {
					session.log.Errorf("No access to call of %s", existingOwner)
					continue
				}
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
						lastCall = customCallID
						session.SetInterest(customCallID, logrus.TraceLevel)
					case "cancel":
						session.log.Infof("Closing call '%s'", customCallID)
						sp.CloseCall(customCallID)
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
			openJobs := make(map[CallID]CallSummary, 0)
			sp.jobs.Range(func(key, value interface{}) bool {
				c := value.(*Call)
				if c.owner == owner {
					openJobs[key.(CallID)] = CallSummary{
						Owner:     c.owner,
						ActorName: c.actorName,
					}
				}
				return true
			})
			session.log.WithField("jobs", openJobs).Infof("jobs started by %s", owner)
			continue
		}

		// TODO: option to change log level per command
		session.SetInterest(callID, logrus.TraceLevel)

		sp.processCmd(actorName, callID, owner, cmdArgs)

		if background {
			lastCall = ""
		} else {
			lastCall = callID
		}
	}
	remaining := session.ListInterests()
	session.log.WithField("calls", remaining).Info("Done, waiting for calls to complete")
	for _, interstCallId := range remaining {
		c, ok := sp.jobs.Load(interstCallId)
		if ok {
			<-c.(*Call).ctx.Done()
		}
	}
}
