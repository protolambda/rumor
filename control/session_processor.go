package control

import (
	"context"
	"fmt"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor"
	"github.com/sirupsen/logrus"
	"strings"
	"sync"
	"time"
)

type SessionProcessor struct {
	adminLog logrus.FieldLogger

	// locks sessionIdCounter and sessions
	sessionsLock     sync.RWMutex
	sessions         map[*Session]struct{}
	sessionIdCounter uint64

	globalActorCtx    context.Context
	globalActorCancel context.CancelFunc

	globalSessionCtx    context.Context
	globalSessionCancel context.CancelFunc

	// a map like map[string]*actor.Actor
	actors sync.Map

	// a map like map[CallID]*Call
	jobs sync.Map
	// logData is a map of all past log data, like map[string]interface{}.
	// Keys are formatted as "{callid}_{entrykey}", i.e. they are concatenated with an underscore.
	// The call ID here excludes the prefix-underscore.
	logData   sync.Map
	log       logrus.FieldLogger
	closeLock sync.Mutex
	closing   bool
}

func NewSessionProcessor(adminLog logrus.FieldLogger) *SessionProcessor {
	log := logrus.New()
	log.SetOutput(VoidWriter{})
	log.SetLevel(logrus.TraceLevel)

	globActCtx, globActCancel := context.WithCancel(context.Background())
	globSessCtx, globSessCancel := context.WithCancel(context.Background())

	sp := &SessionProcessor{
		adminLog:            adminLog,
		sessions:            make(map[*Session]struct{}),
		log:                 log,
		globalActorCtx:      globActCtx,
		globalActorCancel:   globActCancel,
		globalSessionCtx:    globSessCtx,
		globalSessionCancel: globSessCancel,
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
		for k, v := range entry.Data {
			if k == "level" || k == "call_id" || k == "msg" || k == "time" {
				continue
			}
			sp.logData.Store(string(callID)+"_"+k, v)
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

func (sp *SessionProcessor) NewSession(log logrus.FieldLogger) *Session {
	sp.sessionsLock.Lock()
	sp.sessionIdCounter += 1
	s := newSession(SessionID(fmt.Sprintf("s%d", sp.sessionIdCounter)), sp.globalSessionCtx, log, sp)
	sp.sessions[s] = struct{}{}
	sp.sessionsLock.Unlock()
	return s
}

func (sp *SessionProcessor) GetCall(id CallID) *Call {
	dat, ok := sp.jobs.Load(id)
	if !ok {
		return nil
	}
	return dat.(*Call)
}

func (sp *SessionProcessor) IsClosing() bool {
	return sp.closing
}

func (sp *SessionProcessor) GetActor(name actor.ActorID) *actor.Actor {
	a, _ := sp.actors.LoadOrStore(name, actor.NewActor(sp.globalActorCtx, name))
	return a.(*actor.Actor)
}

func (sp *SessionProcessor) KillActor(id actor.ActorID) {
	// get actor
	a, ok := sp.actors.Load(id)
	if ok {
		// if there was an old one, close it
		a.(*actor.Actor).Close()
		sp.actors.Delete(id)
	}
}

func (sp *SessionProcessor) GetLogData(key string) (value interface{}, ok bool) {
	value, ok = sp.logData.Load(key)
	return
}

func (sp *SessionProcessor) ClearLogData() {
	openCalls := make(map[CallID]struct{})
	sp.jobs.Range(func(key, value interface{}) bool {
		k := key.(CallID)
		openCalls[k] = struct{}{}
		return true
	})
	sp.logData.Range(func(key, value interface{}) bool {
		k := key.(string)
		keep := false
		// ranging is fine, open calls should be small, and this doesn't run often
		for id := range openCalls {
			if strings.HasPrefix(k, string(id)) {
				keep = true
				break
			}
		}
		if !keep {
			sp.logData.Delete(k)
		}
		return true
	})
}

func (sp *SessionProcessor) MakeCall(actorName actor.ActorID, callID CallID, cmdArgs []string) *Call {
	rep := sp.GetActor(actorName)
	freeCtx, freeCancel := context.WithCancel(rep.ActorCtx)
	doneCtx, doneCancel := context.WithCancel(freeCtx)
	cmdCtx, cmdCancel := context.WithCancel(doneCtx)
	spawnCtx, spawnCancel := context.WithCancel(freeCtx)

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
		spawned:    false,
		exitReason: SuccessDone,
	}

	callCmd := rep.MakeCmd(cmdLogger, call.Spawn)

	cmdLogger.WithField("args", cmdArgs).Trace("Started")

	loadedCmd, err := ask.Load(callCmd)
	if err != nil {
		cmdLogger.WithError(err).Error("failed to parse command")
		call.exitReason = ParseError
		freeCancel()
		sp.RemoveInterests(callID)
	} else {
		sp.jobs.Store(callID, call)
		go func() {
			fCmd, isHelp, err := loadedCmd.Execute(cmdCtx, cmdArgs...)
			if err != nil {
				cmdLogger.WithError(err).Error("exited with error")
				call.exitReason = RuntimeError
			} else {
				call.exitReason = SuccessDone
				if isHelp {
					cmdLogger.Info(fCmd.Usage())
				}
				cmdLogger.WithField("success", "true").Trace("completed call")
			}
			doneCancel()
			// If nothing was spawned, we can free the command early
			if !call.spawned {
				freeCancel() // just cancel, no need to wait, nothing is blocked
			} else {
				// Waiting for background tasks to be freed
				<-freeCtx.Done()
			}
			// Finished, including optional spawned resources, removing call now
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

func (sp *SessionProcessor) GetJobs(id actor.ActorID) map[CallID]CallSummary {
	openJobs := make(map[CallID]CallSummary, 0)
	sp.jobs.Range(func(key, value interface{}) bool {
		c := value.(*Call)
		if c.actorName == id {
			openJobs[key.(CallID)] = CallSummary{
				Args:      c.args,
				ActorName: c.actorName,
			}
		}
		return true
	})
	return openJobs
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

	sp.log.Debug("Closing remaining sessions...")
	sp.globalSessionCancel()

	sp.log.Debug("Closed session processor")
}
