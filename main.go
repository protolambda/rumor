package main

import (
	"bufio"
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

				go runCommands(log, l.Readline, true)

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

	processCmd := func(actorName string, callID string, cmdArgs []string) {
		rep := getActor(actorName)

		cmdLogger := actor.NewLogger(log.WithField("actor", actorName).WithField("call_id", callID))
		replCmd := rep.Cmd(cmdLogger)

		replCmd.SetOut(WriteableFn(func(msg string) {
			cmdLogger.Info(msg)
		}))
		replCmd.SetErr(WriteableFn(func(msg string) {
			cmdLogger.Error(msg)
		}))
		replCmd.SetArgs(cmdArgs)
		if err := replCmd.Execute(); err != nil {
			cmdLogger.Errorf("Command error: %v\n", err)
		} else {
			// if not interactive, we need to make the caller aware of completion of the command.
			if !interactive {
				cmdLogger.WithField("@success", "").Info("completed call")
			}
		}
	}

	// count calls, for unique ID (if user does not specify their own ID for the call)
	callCounter := 0
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
			log.Errorf("Failed to parse command: %v\n", err)
			continue
		}
		if len(cmdArgs) == 0 {
			continue
		}

		var callID string
		if firstArg := cmdArgs[0]; strings.HasSuffix(firstArg, ">") {
			callID = "@" + firstArg[:len(firstArg)-1]
			cmdArgs = cmdArgs[1:]
		} else {
			callID = fmt.Sprintf("#%d", callCounter)
			callCounter++
		}

		if len(cmdArgs) == 0 {
			continue
		}

		actorName := "DEFAULT_ACTOR"
		if firstArg := cmdArgs[0]; strings.HasSuffix(firstArg, ":") {
			actorName = firstArg[:len(firstArg)-1]
			cmdArgs = cmdArgs[1:]
		}

		processCmd(actorName, callID, cmdArgs)
	}

	// close all libp2p hosts
	for _, actorRep := range actors {
		actorRep.Cancel()
	}
}
