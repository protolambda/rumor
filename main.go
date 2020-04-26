package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/chzyer/readline"
	"github.com/google/shlex"
	"github.com/protolambda/rumor/actor"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

const LogKeyActor = "actor"
const LogKeyCallID = "call_id"

func filterInput(r rune) (rune, bool) {
	switch r {
	// block CtrlZ feature
	case readline.CharCtrlZ:
		return r, false
	}
	return r, true
}

type LogFormatter struct {
	EntryFmtFn
}

type EntryFmtFn func(entry *logrus.Entry) (string, error)

func (fn EntryFmtFn) Format(entry *logrus.Entry) ([]byte, error) {
	out, err := fn(entry)
	return []byte(out), err
}

func (l LogFormatter) WithKeyFormat(
	key string,
	fmtFn func(v interface{}, inner string) (string, error)) LogFormatter {
	return LogFormatter{
		EntryFmtFn: func(entry *logrus.Entry) (string, error) {
			value, hasValue := entry.Data[key]
			if !hasValue {
				return l.EntryFmtFn(entry)
			}
			delete(entry.Data, key)
			defer func() {
				if hasValue {
					entry.Data[key] = value
				}
			}()
			out, err := l.EntryFmtFn(entry)
			if err != nil {
				return "", err
			}
			return fmtFn(value, out)
		},
	}
}

func simpleLogFmt(fmtStr string) func(v interface{}, inner string) (s string, err error) {
	return func(v interface{}, inner string) (s string, err error) {
		return fmt.Sprintf(fmtStr, v) + inner, nil
	}
}

func main() {
	var interactive bool
	fromFilepath := ""
	var level string

	log := logrus.New()
	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.TraceLevel)

	mainCmd := cobra.Command{
		Use:   "rumor",
		Short: "Start Rumor",
		Args: func(cmd *cobra.Command, args []string) error {
			if level != "" {
				logLevel, err := logrus.ParseLevel(level)
				if err != nil {
					return err
				}
				log.SetLevel(logLevel)
			}
			if interactive {
				if len(args) != 0 {
					return fmt.Errorf("interactive mode cannot process any arguments. Got: %s", strings.Join(args, " "))
				}
				coreLogFmt := logrus.TextFormatter{ForceColors: true, DisableTimestamp: true}
				logFmt := LogFormatter{EntryFmtFn: func(entry *logrus.Entry) (string, error) {
					out, err := coreLogFmt.Format(entry)
					if out == nil {
						out = []byte{}
					}
					return string(out), err
				}}
				logFmt = logFmt.WithKeyFormat(LogKeyCallID, simpleLogFmt("\033[33m[%s]\033[0m")) // yellow
				logFmt = logFmt.WithKeyFormat(LogKeyActor, simpleLogFmt("\033[36m[%s]\033[0m"))  // cyan

				log.SetFormatter(logFmt)
			} else {
				if len(args) > 1 {
					return fmt.Errorf("non-interactive mode cannot have more than 1 argument. Got: %s", strings.Join(args, " "))
				}
				if len(args) == 1 {
					fromFilepath = args[0]
				}
				log.SetFormatter(&logrus.JSONFormatter{})
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			if interactive {
				l, err := readline.NewEx(&readline.Config{
					Prompt:              "\033[31m»\033[0m ",
					HistoryFile:         "/tmp/rumor-history.tmp",
					InterruptPrompt:     "^C",
					EOFPrompt:           "exit",
					HistorySearchFold:   true,
					FuncFilterInputRune: filterInput,
				})
				if err != nil {
					log.Error(err)
					return
				}
				defer l.Close()

				sp := NewSessionProcessor(log)
				<-sp.NewSession(log, l.Readline).Done()
				sp.Close()
			} else {
				r := io.Reader(os.Stdin)
				if fromFilepath != "" {
					inputFile, err := os.Open(fromFilepath)
					if err != nil {
						log.Error(err)
						return
					}
					r = inputFile
					defer inputFile.Close()
				}
				sc := bufio.NewScanner(r)
				nextLine := func() (s string, err error) {
					hasMore := sc.Scan()
					text := sc.Text()
					err = sc.Err()
					if err == nil && !hasMore {
						err = io.EOF
					}
					return text, err
				}
				sp := NewSessionProcessor(log)
				<-sp.NewSession(log, nextLine).Done()
				sp.Close()
			}

		},
	}
	mainCmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Interactive mode: run as REPL.")
	mainCmd.Flags().StringVar(&level, "level", "trace", "Log-level. Valid values: trace, debug, info, warn, error, fatal, panic")

	if err := mainCmd.Execute(); err != nil {
		log.Error(err)
	}
	os.Exit(0)
}

type LogSplitFn func(entry *logrus.Entry) error

func (fn LogSplitFn) Format(entry *logrus.Entry) ([]byte, error) {
	// we don't care about formatting the message contents, we just forward the entry itself elsewhere
	return nil, fn(entry)
}

type SessionID string

type Session struct {
	sessionID    SessionID
	interests    map[CallID]logrus.Level
	log          logrus.FieldLogger
	readNextLine func() (string, error)
	ctx          context.Context
}

func (s *Session) Done() <-chan struct{} {
	return s.ctx.Done()
}

type SessionProcessor struct {
	adminLog         logrus.FieldLogger
	sessions         map[*Session]struct{}
	sessionIdCounter uint64


	// TODO change to concurrency safe maps
	actors    map[string]*actor.Actor
	jobs      map[CallID]*Call
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
		actors: make(map[string]*actor.Actor),
		jobs:   make(map[CallID]*Call),
		log:    log,
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
			if lvl, ok := s.interests[callID]; ok {
				if lvl >= entry.Level {
					s.log.WithFields(entry.Data).Log(entry.Level, entry.Message)
				}
			}
		}
		return nil
	}))

	return sp
}

func (sp *SessionProcessor) NewSession(log logrus.FieldLogger, readNextLine func() (string, error)) *Session {
	// TODO make this concurrency safe
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

	go func(sh *SessionProcessor, cancel context.CancelFunc, s *Session) {
		sh.runSession(s)
		// Declare the the session closed as soon as it exits
		cancel()
		delete(sp.sessions, s)
	}(sp, cancel, s)

	return s
}

type WriteableFn func(msg string)

func (fn WriteableFn) Write(p []byte) (n int, err error) {
	fn(string(p))
	return len(p), nil
}

type CallID string

type Call struct {
	ctx    context.Context
	cancel context.CancelFunc
	logger *actor.Logger
}

func (sp *SessionProcessor) GetActor(name string) *actor.Actor {
	if a, ok := sp.actors[name]; ok {
		return a
	} else {
		rep := actor.NewActor()
		sp.actors[name] = rep
		return rep
	}
}

func (sp *SessionProcessor) processCmd(actorName string, callID CallID, cmdArgs []string) {
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
	}

	sp.jobs[callID] = call

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
	c, ok := sp.jobs[id]
	if !ok {
		return
	}
	c.cancel()
	delete(sp.jobs, id)
	// TODO concurrency fix
	for s := range sp.sessions {
		delete(s.interests, id)
	}
	c.logger.Info("Closed call")
}

func (sp *SessionProcessor) Close() {
	sp.closeLock.Lock()
	defer sp.closeLock.Unlock()
	sp.closing = true

	var wg sync.WaitGroup
	// close remaining jobs
	for k, v := range sp.jobs {
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
	}
	wg.Wait()

	// close all libp2p hosts
	for _, actorRep := range sp.actors {
		actorRep.Close()
	}
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

		if lastCall != "" {
			_, ok := sp.jobs[lastCall]
			if ok {
				if line == "cancel" {
					sp.CloseCall(lastCall)
				} else if line == "bg" {
					session.log.Infof("Moved call '%s' to background", lastCall)
					lastCall = ""
				} else {
					session.log.Errorf("Unrecognized command for modifying last call: '%s'", line)
				}
				continue
			} else {
				lastCall = ""
			}
		}

		cmdArgs, err := shlex.Split(line)
		if err != nil {
			session.log.Errorf("Failed to parse command: %v\n", err)
			continue
		}
		if len(cmdArgs) == 0 {
			continue
		}

		var callID CallID
		if firstArg := cmdArgs[0]; strings.HasSuffix(firstArg, ">") {
			callID = CallID(firstArg[:len(firstArg)-1])
			cmdArgs = cmdArgs[1:]
		} else {
			callID = CallID(fmt.Sprintf("%s_%d", session.sessionID, callCounter))
			callCounter++
		}

		if len(cmdArgs) == 0 {
			continue
		}

		// try historical call if there is no current call
		if _, ok := sp.jobs[callID]; ok {
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

		actorName := "DEFAULT_ACTOR"
		if firstArg := cmdArgs[0]; strings.HasSuffix(firstArg, ":") {
			actorName = firstArg[:len(firstArg)-1]
			cmdArgs = cmdArgs[1:]
		}

		if len(cmdArgs) == 0 {
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
		session.interests[callID] = logrus.TraceLevel

		sp.processCmd(actorName, callID, cmdArgs)

		if background {
			lastCall = ""
		} else {
			lastCall = callID
		}
	}
}
