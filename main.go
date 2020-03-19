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
	"os/signal"
	"strings"
	"syscall"
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
	log.SetLevel(logrus.InfoLevel)

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
				logFmt = logFmt.WithKeyFormat(LogKeyActor, simpleLogFmt("\033[36m[%s]\033[0m")) // cyan

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
				stop := make(chan os.Signal, 1)
				signal.Notify(stop, syscall.SIGINT)

				go runCommands(log, l.Readline,true)

				<-stop
				log.Info("Exiting...")
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
				runCommands(log, nextLine, false)
			}

		},
	}
	mainCmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Interactive mode: run as REPL.")
	mainCmd.Flags().StringVar(&level, "level", "info", "Log-level. Valid values: trace, debug, info, warn, error, fatal, panic")

	if err := mainCmd.Execute(); err != nil {
		log.Error(err)
	}
	os.Exit(0)
}

type WriteableFn func(msg string)

func (fn WriteableFn) Write(p []byte) (n int, err error) {
	fn(string(p))
	return len(p), nil
}

type CallID string

type Call struct {
	ctx context.Context
	actorName string
	cancel context.CancelFunc
	logger *actor.Logger
	isDone bool
}

func (c *Call) Close() {
	c.isDone = true
	c.cancel()
}

func runCommands(log logrus.FieldLogger, nextLine func() (string, error), interactive bool) {
	actors := make(map[string]*actor.Actor)

	getActor := func(name string) *actor.Actor {
		if a, ok := actors[name]; ok {
			return a
		} else {
			rep := actor.NewActor()
			actors[name] = rep
			return rep
		}
	}

	processCmd := func(actorName string, callID CallID, cmdArgs []string) *Call {
		rep := getActor(actorName)
		cmdCtx, cmdCancel := context.WithCancel(rep.ActorCtx)

		cmdLogger := actor.NewLogger(log.WithField("actor", actorName).WithField("call_id", callID))
		replCmd := rep.Cmd(cmdCtx, cmdLogger)

		replCmd.SetOut(WriteableFn(func(msg string) {
			cmdLogger.Info(msg)
		}))
		replCmd.SetErr(WriteableFn(func(msg string) {
			cmdLogger.Error(msg)
		}))
		replCmd.SetArgs(cmdArgs)

		call := &Call{
			ctx:       cmdCtx,
			actorName: actorName,
			cancel:    cmdCancel,
			logger:    cmdLogger,
		}

		go func() {
			if err := replCmd.Execute(); err != nil {
				cmdLogger.Error(err) // TODO: cobra error output sometimes is written to std-out. Need it in std-err to detect it as error.
				// For now, take the execute result, and use that instead. (probably better, but still need to throw std-err of cobra somewhere)
			} else {
				// if not interactive, we need to make the caller aware of completion of the command.
				if !interactive {
					cmdLogger.WithField("@success", "").Info("completed call")
				}
			}
			call.Close()
		}()
		return call
	}

	loopCtx, loopCancel := context.WithCancel(context.Background())
	lines := make(chan string, 1)
	go func() {
		for {
			line, err := nextLine()
			if err == readline.ErrInterrupt {
				if len(line) == 0 {
					break
				} else {
					continue
				}
			} else if err == io.EOF {
				break
			}
			lines <- line
		}
		loopCancel()
	}()

	var lastCall *Call = nil

	jobs := make(map[CallID]*Call)

	// count calls, for unique ID (if user does not specify their own ID for the call)
	callCounter := 0
	mainLoop: for {
		// remove done jobs
		for k, v := range jobs {
			if v.isDone {
				delete(jobs, k)
			}
		}
		var lastDone <-chan struct{} = nil
		if lastCall != nil {
			lastDone = lastCall.ctx.Done()
		}
		line := ""
		select {
		case <- loopCtx.Done():
			break mainLoop
		case v := <-lines:
			line = v
		//noinspection GoNilness
		case <- lastDone: // waits forever if nil, other select gets hit
			lastCall = nil
			continue
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
			loopCancel()
			break
		}

		if lastCall != nil {
			if line == "cancel" {
				lastCall.logger.Info("Canceled call")
				lastCall.Close()
				continue
			} else if line == "bg" {
				lastCall.logger.Info("Moved call to background")
				lastCall = nil
				continue
			} else {
				lastCall.logger.Errorf("Unrecognized command for modifying last call: '%s'", line)
			}
		}

		cmdArgs, err := shlex.Split(line)
		if err != nil {
			log.Errorf("Failed to parse command: %v\n", err)
			continue
		}
		if len(cmdArgs) == 0 {
			continue
		}

		var callID CallID
		if firstArg := cmdArgs[0]; strings.HasSuffix(firstArg, ">") {
			callID = CallID("@" + firstArg[:len(firstArg)-1])
			cmdArgs = cmdArgs[1:]
		} else {
			callID = CallID(fmt.Sprintf("#%d", callCounter))
			callCounter++
		}

		if len(cmdArgs) == 0 {
			continue
		}

		// try historical call if there is no current call
		if call, ok := jobs[callID]; ok {
			if len(cmdArgs) == 1 && cmdArgs[0] == "fg" {
				call.logger.Info("Moved call to foreground")
				lastCall = call
			} else if len(cmdArgs) == 1 && cmdArgs[0] == "cancel" {
				call.logger.Info("Canceled call")
				call.Close()
			} else {
				call.logger.Errorf("Unrecognized command for modifying call: '%s'", line)
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

		call := processCmd(actorName, callID, cmdArgs)
		jobs[callID] = call

		if background {
			lastCall = nil
		} else {
			lastCall = call
		}
	}

	close(lines)

	// close all libp2p hosts
	for _, actorRep := range actors {
		actorRep.Close()
	}
}
