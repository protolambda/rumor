package mngmt

import (
	"context"
	"fmt"
	"github.com/google/shlex"
	"github.com/protolambda/rumor/actor"
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

type CallOwner string

type Call struct {
	ctx    context.Context
	cancel context.CancelFunc
	logger *actor.Logger
	owner  CallOwner
}

func (sp *SessionProcessor) GetActor(name string) *actor.Actor {
	a, _ := sp.actors.LoadOrStore(name, actor.NewActor())
	return a.(*actor.Actor)
}

func (sp *SessionProcessor) processCmd(actorName string, callID CallID, owner CallOwner, cmdArgs []string) {
	rep := sp.GetActor(actorName)
	cmdCtx, cmdCancel := context.WithCancel(rep.ActorCtx)

	cmdLogger := actor.NewLogger(sp.log.WithField("actor", actorName).WithField("call_id", callID))
	callCmd := rep.Cmd(cmdCtx, cmdLogger)

	callCmd.SetOut(WriteableFn(func(msg string) {
		cmdLogger.Info(msg)
	}))
	callCmd.SetErr(WriteableFn(func(msg string) {
		cmdLogger.Error(msg)
	}))
	callCmd.SetArgs(cmdArgs)

	call := &Call{
		ctx:    cmdCtx,
		cancel: cmdCancel,
		logger: cmdLogger,
		owner: owner,
	}

	sp.jobs.Store(callID, call)

	go func() {
		if err := callCmd.Execute(); err != nil {
			cmdLogger.Error(err) // TODO: cobra error output sometimes is written to std-out. Need it in std-err to detect it as error.
			// For now, take the execute result, and use that instead. (probably better, but still need to throw std-err of cobra somewhere)
		} else {
			cmdLogger.WithField("@success", "").Trace("completed call")
		}
		sp.CloseCall(callID)
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
	sp.sessionsLock.RLock()
	for s := range sp.sessions {
		s.UnsetInterest(id)
	}
	sp.sessionsLock.RUnlock()
	sp.log.Infof("Closed call: '%s'", id)
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
			return
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
		var callID CallID
		var owner CallOwner
		{
			var actorStr, callIDStr, ownerStr string
			skip := 0
			for _, arg := range cmdArgs {
				if len(arg) <= 1 {
					break
				}
				if strings.HasSuffix(arg, ":") && actorStr == "" {
					actorStr = arg[:len(arg)-1]
					skip++
				} else if strings.HasSuffix(arg, ">") && callIDStr == "" {
					callIDStr = arg[:len(arg)-1]
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
			if callIDStr == "" {
				if lastCall != "" {
					_, ok := sp.jobs.Load(lastCall)
					if ok {
						callIDStr = string(lastCall)
					} else {
						callIDStr = fmt.Sprintf("%s_%d", session.sessionID, callCounter)
						callCounter++
					}
				} else {
					callIDStr = fmt.Sprintf("%s_%d", session.sessionID, callCounter)
					callCounter++
				}
			}
			if ownerStr == "" {
				ownerStr = "DEFAULT_OWNER"
			}

			actorName = actorStr
			callID = CallID(callIDStr)
			owner = CallOwner(ownerStr)
		}

		if len(cmdArgs) == 0 {
			continue
		}

		if lastCall != "" {
			c, ok := sp.jobs.Load(lastCall)
			if ok {
				lastCallObj := c.(*Call)
				if existingOwner := lastCallObj.owner; existingOwner != owner {
					sp.log.Errorf("No access to call of %s", existingOwner)
					continue
				} else {
					if len(cmdArgs) == 1 && cmdArgs[0] == "cancel" {
						sp.CloseCall(lastCall)
					} else if len(cmdArgs) == 1 && cmdArgs[0] == "bg" {
						session.log.Infof("Moved call '%s' to background", lastCall)
						lastCall = ""
					} else {
						// if a next command, wait for current call to stop
						session.log.Infof("Waiting for call '%s'", callID)
						<-lastCallObj.ctx.Done()

						callID = CallID(fmt.Sprintf("%s_%d", session.sessionID, callCounter))
						callCounter++
					}
				}
			} else {
				// call does not exist anymore, it ended. Safe to do another call with new id.
				lastCall = ""
			}
		}

		// try historical call if there is no current call
		if c, ok := sp.jobs.Load(callID); ok {
			if existingOwner := c.(*Call).owner; existingOwner != owner {
				sp.log.Errorf("No access to call of %s", existingOwner)
				continue
			}
			if len(cmdArgs) == 1 && cmdArgs[0] == "fg" {
				session.log.Infof("Moved call '%s' to foreground", callID)
				lastCall = callID
			} else if len(cmdArgs) == 1 && cmdArgs[0] == "cancel" {
				session.log.Infof("Closing call '%s'", callID)
				sp.CloseCall(callID)
			} else {
				session.log.Errorf("Unrecognized command for modifying call: '%s'", line)
			}
			continue
		}

		background := false
		if cmdArgs[0] == "bg" {
			background = true
			cmdArgs = cmdArgs[1:]
		}

		if len(cmdArgs) == 0 {
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
}
