package control

import (
	"context"
	"fmt"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/chain"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/sirupsen/logrus"
	"io"
	"mvdan.cc/sh/v3/expand"
	"os"
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

	actorGlobals      actor.GlobalActorData
	globalActorCancel context.CancelFunc

	globalSessionCtx    context.Context
	globalSessionCancel context.CancelFunc

	// a map like map[string]*actor.Actor
	actors sync.Map

	mainEnv expand.Environ

	// a map like map[CallID]*Call
	ongoingCalls sync.Map
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
		adminLog: adminLog,
		actorGlobals: actor.GlobalActorData{
			GlobalCtx:        globActCtx,
			GlobalPeerstores: &track.PeerstoresMap{},
			GlobalChains:     &chain.ChainsMap{},
			GlobalBlocksDBs:  &bdb.DBMap{},
			GlobalStatesDBs:  &sdb.DBMap{},
		},
		sessions:            make(map[*Session]struct{}),
		log:                 log,
		mainEnv:             expand.ListEnviron(os.Environ()...),
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
		sp.sessionsLock.RLock()
		defer sp.sessionsLock.RUnlock()
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
	s := newSession(SessionID(fmt.Sprintf("s%d", sp.sessionIdCounter)), sp.globalSessionCtx, sp.mainEnv, log, sp)
	sp.sessions[s] = struct{}{}
	sp.sessionsLock.Unlock()
	return s
}

func (sp *SessionProcessor) NewSubSession(log logrus.FieldLogger, ctx context.Context, parentEnv expand.Environ) *Session {
	sp.sessionsLock.Lock()
	sp.sessionIdCounter += 1
	s := newSession(SessionID(fmt.Sprintf("s%d", sp.sessionIdCounter)), ctx, parentEnv, log, sp)
	sp.sessions[s] = struct{}{}
	sp.sessionsLock.Unlock()
	return s
}

func (sp *SessionProcessor) GetCall(id CallID) *Call {
	dat, ok := sp.ongoingCalls.Load(id)
	if !ok {
		return nil
	}
	return dat.(*Call)
}

func (sp *SessionProcessor) IsClosing() bool {
	return sp.closing
}

func (sp *SessionProcessor) GetActor(name actor.ActorID) *actor.Actor {
	// try optimistically loading first, don't create a new actor just yet.
	a, ok := sp.actors.Load(name)
	if ok {
		return a.(*actor.Actor)
	} else {
		// if not already there, create the actor, but avoid overwriting if getting actors concurrently.
		a, _ := sp.actors.LoadOrStore(name, actor.NewActor(name, &sp.actorGlobals))
		return a.(*actor.Actor)
	}
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
	sp.ongoingCalls.Range(func(key, value interface{}) bool {
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

func (sp *SessionProcessor) MakeCall(callCtx context.Context, out io.Writer,
	actorName actor.ActorID, callID CallID, cmdArgs []string) (*Call, error) {

	rep := sp.GetActor(actorName)
	bgCtx, bgCancel := context.WithCancel(rep.ActorCtx)

	cmdLogger := sp.log.WithField("actor", actorName).WithField("call_id", callID)

	call := &Call{
		id:          callID,
		args:        cmdArgs,
		startTimeNS: time.Now().UnixNano(),
		steps:       make(chan base.Step, 0),
		bgCtx:       bgCtx,
		bgCancel:    bgCancel,
		logger:      cmdLogger,
		actorName:   actorName,
		spawned:     false,
	}

	callCmd := rep.MakeCmd(cmdLogger, call, out)

	sp.log.WithField("args", cmdArgs).Tracef("Started %s", callID)

	loadedCmd, err := ask.Load(callCmd)
	if err != nil {
		bgCancel()
		sp.RemoveInterests(callID)
		return nil, fmt.Errorf("failed to parse command: %v", err)
	} else {
		sp.ongoingCalls.Store(callID, call)

		defer func() {
			go func() {
				// If nothing was spawned, we can bgCancel the command early
				if !call.spawned {
					bgCancel() // just cancel, no need to wait, nothing is blocked
				} else {
					// Waiting for background tasks to be freed
					<-bgCtx.Done()
				}
				cmdLogger.WithField("__freed", "true").Trace("freed call resources")
				// Finished, including optional spawned resources, removing call now
				sp.RemoveInterests(callID)
				sp.ongoingCalls.Delete(callID)
			}()
		}()

		fCmd, isHelp, err := loadedCmd.Execute(callCtx, cmdArgs...)
		if err != nil {
			cmdLogger.WithField("__error", err).Trace("call failed with error")
			return call, fmt.Errorf("command failed: %v", err)
		} else {
			if isHelp {
				cmdLogger.Info(fCmd.Usage())
			}
			cmdLogger.WithField("__success", "true").Trace("completed call")
			return call, nil
		}
	}
}

func (sp *SessionProcessor) RemoveInterests(id CallID) {
	sp.sessionsLock.RLock()
	for s := range sp.sessions {
		s.UnsetInterest(id)
	}
	sp.sessionsLock.RUnlock()
}

func (sp *SessionProcessor) GetCalls(id actor.ActorID) map[CallID]CallSummary {
	openJobs := make(map[CallID]CallSummary, 0)
	sp.ongoingCalls.Range(func(key, value interface{}) bool {
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

type ReportActor struct {
	// May be empty if no host is active
	PeerID      string   `json:"peer_id,omitempty"`
	ListenAddrs []string `json:"listen_addrs,omitempty"`
	Peers       []string `json:"peers,omitempty"`
	ENR         string   `json:"enr,omitempty"`
	// TODO: could add more peer info
}

type ReportCall struct {
	Actor       actor.ActorID `json:"actor,omitempty"`
	Args        []string      `json:"args,omitempty"`
	StartTimeMS int64         `json:"start_time,omitempty"`
}

type Report struct {
	Actors map[actor.ActorID]ReportActor `json:"actors"`
	Calls  map[CallID]ReportCall         `json:"calls"`
}

func (sp *SessionProcessor) Report() *Report {
	rep := &Report{
		Actors: make(map[actor.ActorID]ReportActor),
		Calls:  make(map[CallID]ReportCall),
	}
	sp.actors.Range(func(key, value interface{}) bool {
		id := key.(actor.ActorID)
		ac := value.(*actor.Actor)
		repAc := ReportActor{}
		h, err := ac.HostState.Host()
		if err == nil {
			repAc.PeerID = h.ID().String()
			for _, a := range h.Addrs() {
				repAc.ListenAddrs = append(repAc.ListenAddrs, a.String())
			}
			for _, p := range h.Network().Peers() {
				repAc.Peers = append(repAc.Peers, p.String())
			}
		}
		if c := ac.LazyEnrState.Current; c != nil {
			repAc.ENR = c.GetNode().String()
		}
		rep.Actors[id] = repAc
		return true
	})
	sp.ongoingCalls.Range(func(key, value interface{}) bool {
		id := key.(CallID)
		call := value.(*Call)
		rep.Calls[id] = ReportCall{
			Actor:       call.actorName,
			Args:        call.args,
			StartTimeMS: call.startTimeNS / 1000,
		}
		return true
	})
	return rep
}

func (sp *SessionProcessor) Close() {
	sp.closeLock.Lock()
	defer sp.closeLock.Unlock()
	sp.closing = true

	var wg sync.WaitGroup
	log := sp.log.(logrus.Ext1FieldLogger)
	log.Trace("Closing remaining calls...")
	// close remaining calls
	sp.ongoingCalls.Range(func(ki, vi interface{}) bool {
		v := vi.(*Call)
		v.logger.Debug("Closing job with 5 second timeout...")
		wg.Add(1)
		closeCtx, _ := context.WithTimeout(v.bgCtx, time.Second*5)
		if err := v.RequestStop(closeCtx); err != nil {
			v.logger.Error("Failed to close job with timeout")
		}
		wg.Done()
		return true
	})
	wg.Wait()

	log.Trace("Closing remaining actors...")
	// close all libp2p hosts
	sp.actors.Range(func(key, value interface{}) bool {
		value.(*actor.Actor).Close()
		return true
	})

	log.Trace("Closing global context...")
	// closes cross-actor things such as peerstores and chains
	sp.globalActorCancel()

	log.Trace("Closing remaining sessions...")
	sp.globalSessionCancel()

	log.Trace("Closed session processor")
}
