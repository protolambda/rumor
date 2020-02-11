package main

import (
	"eth2-lurk/repl"
	"fmt"
	"github.com/chzyer/readline"
	"github.com/sirupsen/logrus"
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
	out, err := l.TextFormatter.Format(entry)
	if err != nil {
		return nil, err
	}
	if okTopic {
		return []byte(fmt.Sprintf("\033[34m[%10s]\033[0m %s", topic, out)), nil
	} else {
		return out, nil
	}
}

func main() {
	l, err := readline.NewEx(&readline.Config{
		Prompt:          "\033[31m»\033[0m ",
		HistoryFile:     "/tmp/eth2-network-repl.tmp",
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",

		HistorySearchFold:   true,
		FuncFilterInputRune: filterInput,
	})
	if err != nil {
		panic(err)
	}

	log := logrus.New()
	log.SetFormatter(&LogFormatter{TextFormatter: logrus.TextFormatter{ForceColors: true}})
	log.SetOutput(l.Stderr())
	log.SetLevel(logrus.TraceLevel)

	rep := repl.NewRepl(log)
	rep.ReplCmd.SetOut(l.Stdout())
	rep.ReplCmd.SetErr(l.Stderr())

	stop := make(chan os.Signal, 1)
	go func() {
		for {
			line, err := l.Readline()
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
			if line == "exit" {
				stop <- syscall.SIGINT
				break
			}
			cmdArgs := strings.Fields(line)
			rep.ReplCmd.SetArgs(cmdArgs)
			if err := rep.ReplCmd.Execute(); err != nil {
				_, _ = fmt.Fprintln(os.Stderr, err)
			}
		}
	}()

	signal.Notify(stop, syscall.SIGINT)

	<-stop
	log.Info("Exiting...")
	_ = l.Close()
	rep.Cancel()
	os.Exit(0)
}
