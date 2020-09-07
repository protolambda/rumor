package sh

import (
	"bufio"
	"encoding/json"
	"github.com/chzyer/readline"
	"github.com/gorilla/websocket"
	"github.com/protolambda/rumor/control"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"strings"
	"time"
)

// Stop listening to users that don't give us commands for an hour
const UserReadTimeout = time.Hour * 1

// Stop writing to users that can't receive a command within 10 seconds.
const UserWriteTimeout = time.Second * 10

func filterInput(r rune) (rune, bool) {
	switch r {
	// block CtrlZ feature
	case readline.CharCtrlZ:
		return r, false
	}
	return r, true
}

type TimedNetOut struct {
	c       net.Conn
	timeout time.Duration
}

func (t *TimedNetOut) Write(b []byte) (n int, err error) {
	if err := t.c.SetWriteDeadline(time.Now().Add(t.timeout)); err != nil {
		return 0, err
	}
	return t.c.Write(b)
}

type TimedWsOut struct {
	c       *websocket.Conn
	timeout time.Duration
}

func (t *TimedWsOut) Write(b []byte) (n int, err error) {
	if err := t.c.SetWriteDeadline(time.Now().Add(t.timeout)); err != nil {
		return 0, err
	}
	return len(b), t.c.WriteMessage(websocket.TextMessage, b)
}

// Every time some input has been parsed, call-back if parsing got a complete statement. If so, the full data is returned.
type ParseBufferFn func(complete bool, fail bool) string

const startPrompt = "\033[31m»\033[0m "
const multilinePrompt = "\033[31m>>> \033[0m "

// multi-line inputs end each intermediate line with "\"
func shellLogger(levelFlag string, w io.Writer) (logrus.FieldLogger, error) {
	log := logrus.New()
	log.SetOutput(w)
	log.SetLevel(logrus.TraceLevel)
	log.SetFormatter(&control.ShellLogFmt{})

	if levelFlag != "" {
		logLevel, err := logrus.ParseLevel(levelFlag)
		if err != nil {
			return nil, err
		}
		log.SetLevel(logLevel)
	}
	return log, nil
}

func shellReader() (*readline.Instance, error) {
	return readline.NewEx(&readline.Config{
		Prompt:                 startPrompt,
		HistoryFile:            "/tmp/rumor-history.tmp",
		InterruptPrompt:        "^C",
		EOFPrompt:              "exit",
		HistorySearchFold:      true,
		FuncFilterInputRune:    filterInput,
		DisableAutoSaveHistory: true, // disabled to deal with multi-line history better
	})
}

func shellMode(rl *readline.Instance,
	run func(nextLine control.NextMultiLineFn, onParse ParseBufferFn)) {
	defer rl.Close()

	// re-use buffer, tracking multi-line history
	var cmdBuf []string
	onParse := func(complete bool, fail bool) string {
		if fail { // no history saved
			// reset data
			cmdBuf = cmdBuf[:0]
			// prepare for next command
			rl.SetPrompt(startPrompt)
			return ""
		}
		if complete {
			withSeparator := strings.Join(cmdBuf, "; ")
			multiLine := strings.Join(cmdBuf, "\n")
			// reset data
			cmdBuf = cmdBuf[:0]
			// best effort to fit the command into history as a valid shell command,
			// the readline library has a bug with line breaks in history
			_ = rl.SaveHistory(withSeparator)
			// prepare for next command
			rl.SetPrompt(startPrompt)
			return multiLine
		} else {
			// just continue parsing
			rl.SetPrompt(multilinePrompt)
			return ""
		}
	}
	readMultiLine := control.NextMultiLineFn(func() (string, error) {
		for {
			line, err := rl.Readline()
			if err != nil {
				return "", err
			}
			// If it's the first line, and it's empty, then nothing happens.
			if len(cmdBuf) == 0 && strings.TrimSpace(line) == "" {
				continue
			}
			cmdBuf = append(cmdBuf, line)
			return line, nil
		}
	})
	run(readMultiLine, onParse)
}

func feedLog(log logrus.FieldLogger, nextLogLine control.NextSingleLineFn, stopped *bool) {
	for {
		line, err := nextLogLine()
		if err != nil {
			// If we reach the end, or if we needed to stop,
			// then we expect no line can be read, and the error can be ignored.
			if err != io.EOF && !*stopped {
				log.Error(err)
			}
			return
		}
		var entry logrus.Fields
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			log.Error(err)
			continue
		}
		leveli, ok := entry["level"]
		if !ok {
			leveli = "info"
		}
		levelStr, ok := leveli.(string)
		delete(entry, "level")
		lvl, err := logrus.ParseLevel(levelStr)
		if err != nil {
			log.Error(err)
			continue
		}
		msg, ok := entry["msg"]
		if !ok {
			msg = nil
		}
		delete(entry, "msg")
		delete(entry, "time")
		log.WithFields(entry).Log(lvl, msg)
	}
}

func asLineReader(r io.Reader) control.NextSingleLineFn {
	sc := bufio.NewScanner(r)
	return func() (s string, err error) {
		hasMore := sc.Scan()
		text := sc.Text()
		err = sc.Err()
		if err == nil && !hasMore {
			err = io.EOF
		}
		return text, err
	}
}
