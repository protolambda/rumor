package main

import (
	"bufio"
	"fmt"
	"github.com/chzyer/readline"
	"github.com/google/shlex"
	"github.com/protolambda/rumor/repl"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
)


func filterInput(r rune) (rune, bool) {
	switch r {
	// block CtrlZ feature
	case readline.CharCtrlZ:
		return r, false
	}
	return r, true
}

type LogFormatter struct {
	logrus.TextFormatter
}

func (l *LogFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	topic, okTopic := entry.Data["log_topic"]
	delete(entry.Data, "log_topic")
	actor, okActor := entry.Data["actor"]
	delete(entry.Data, "actor")
	out, err := l.TextFormatter.Format(entry)
	if okTopic {
		entry.Data["log_topic"] = topic
	}
	if okActor {
		entry.Data["actor"] = actor
	}
	if err != nil {
		return nil, err
	}
	if okTopic {
		if okActor {
			return []byte(fmt.Sprintf("\033[34m[%s]\033[35m[%s]\033[0m %s", actor, topic, out)), nil
		} else {
			return []byte(fmt.Sprintf("\033[35m[%s]\033[0m %s", topic, out)), nil
		}
	} else {
		if okActor {
			return []byte(fmt.Sprintf("\033[34m[%s]\033[0m %s", actor, out)), nil
		} else {
			return out, nil
		}
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
				log.SetFormatter(&LogFormatter{TextFormatter: logrus.TextFormatter{ForceColors: true}})
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

				go runCommands(log, l.Stdout(), l.Stderr(), l.Readline)

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
				runCommands(log, os.Stdout, os.Stderr, nextLine)
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

func runCommands(log logrus.FieldLogger, out io.Writer, errOut io.Writer, nextLine func() (string, error)) {
	inputLog := log.WithField("log_topic", "input")
	commentLog := log.WithField("log_topic", "comment")

	actors := make(map[string]*repl.Repl)

	getActor := func(name string) *repl.Repl{
		if actor, ok := actors[name]; ok {
			return actor
		} else {
			rep := repl.NewRepl()
			actors[name] = rep
			return rep
		}
	}

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
			commentLog.Info(line)
			continue
		}
		inputLog.Info(line)
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
		actorName := "DEFAULT_ACTOR"
		if firstArg := cmdArgs[0]; strings.HasSuffix(firstArg, ":") {
			actorName = firstArg[:len(firstArg)-1]
			cmdArgs = cmdArgs[1:]
		}

		rep := getActor(actorName)

		replCmd := rep.Cmd(log.WithField("actor", actorName))
		cmd, remainingArgs, err := replCmd.Find(cmdArgs)
		if err != nil {
			log.Errorf("Failed to find command: %v\n", err)
			continue
		}
		path := strings.Join(cmdArgs[len(cmdArgs) - len(remainingArgs):], "/")
		// TODO: fix log to include command path
		cmd.SetOut(out)
		cmd.SetErr(errOut)
		cmd.SetArgs(remainingArgs)
		if err := cmd.Execute(); err != nil {
			log.Errorf("Command error: %v\n", err)
			continue
		}
	}

	for _, actorRep := range actors {
		actorRep.Cancel()
	}
}
