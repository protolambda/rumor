package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/chzyer/readline"
	"github.com/gorilla/websocket"
	"github.com/protolambda/rumor/control"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"mvdan.cc/sh/v3/syntax"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// Stop listening to users that don't give us commands for an hour
const UserReadTimeout = time.Hour * 1

// Stop writing to users that can't receive a command within 10 seconds.
const UserWriteTimeout = time.Second * 10

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

func main() {

	mainCmd := cobra.Command{
		Use:   "rumor",
		Short: "Start Rumor",
	}

	// multi-line inputs end each intermediate line with "\"
	shellMode := func(levelFlag string, run func(log logrus.FieldLogger, nextLine control.NextMultiLineFn)) {
		log := logrus.New()
		log.SetOutput(os.Stdout)
		log.SetLevel(logrus.TraceLevel)

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

		if levelFlag != "" {
			logLevel, err := logrus.ParseLevel(levelFlag)
			if err != nil {
				log.Error(err)
			}
			log.SetLevel(logLevel)
		}
		startPrompt := "\033[31m»\033[0m "
		multilinePrompt := "\033[31m>>> \033[0m "

		rl, err := readline.NewEx(&readline.Config{
			Prompt:                 startPrompt,
			HistoryFile:            "/tmp/rumor-history.tmp",
			InterruptPrompt:        "^C",
			EOFPrompt:              "exit",
			HistorySearchFold:      true,
			FuncFilterInputRune:    filterInput,
			DisableAutoSaveHistory: true, // disabled to deal with multi-line history better
		})
		if err != nil {
			log.Error(err)
			return
		}
		defer rl.Close()
		// re-use commands buffer
		var cmds []string
		readMultiLine := control.NextMultiLineFn(func() (string, error) {
			for {
				line, err := rl.Readline()
				if err != nil {
					return "", err
				}
				line = strings.TrimSpace(line)
				if len(line) == 0 {
					continue
				}
				cmds = append(cmds, line)
				if strings.HasSuffix(line, " \\") {
					rl.SetPrompt(multilinePrompt)
					continue
				}
				cmd := strings.Join(cmds, " ")
				cmds = cmds[:0]
				rl.SetPrompt(startPrompt)
				_ = rl.SaveHistory(cmd)
				return cmd, nil
			}
		})
		run(log, readMultiLine)
	}

	feedLog := func(log logrus.FieldLogger, nextLogLine control.NextSingleLineFn, stopped *bool) {
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
			log.WithFields(entry).Log(lvl, msg)
		}
	}

	asLineReader := func(r io.Reader) control.NextSingleLineFn {
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

	{
		var wsAddr string
		var ipcPath string
		var tcpAddr string
		var wsApiKey string

		var level string
		attachCmd := &cobra.Command{
			Use:   "attach",
			Short: "Attach to a running rumor server",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				shellMode(level, func(log logrus.FieldLogger, nextLine control.NextMultiLineFn) {
					var nextLogLine control.NextSingleLineFn
					var writeLine func(v string) error
					if wsAddr != "" {
						u, err := url.Parse(wsAddr)
						if err != nil {
							log.Fatalf("failed to parse websocket url: '%s', err: %v", wsAddr, err)
							return
						}
						h := http.Header{}
						if wsApiKey != "" {
							h["X-Api-Key"] = []string{wsApiKey}
						}
						conn, _, err := websocket.DefaultDialer.Dial(u.String(), h)
						if err != nil {
							log.Fatalf("WS dial error: %v", err)
						}
						nextLogLine = func() (string, error) {
							mType, mContent, err := conn.ReadMessage()
							if err != nil {
								return "", err
							}
							if mType != websocket.TextMessage {
								return "", errors.New("expected text-type websocket messages")
							}
							return string(mContent), nil
						}
						writeLine = func(v string) error {
							return conn.WriteMessage(websocket.TextMessage, []byte(v+"\n"))
						}
						defer conn.Close()
					} else if ipcPath != "" || tcpAddr != "" {
						var netInput net.Conn
						var err error
						if ipcPath != "" {
							netInput, err = net.Dial("unix", ipcPath)
							if err != nil {
								log.Fatal("IPC dial error: ", err)
							}
						} else if tcpAddr != "" {
							netInput, err = net.Dial("tcp", tcpAddr)
							if err != nil {
								log.Fatal("TCP dial error: ", err)
							}
						}
						nextLogLine = asLineReader(netInput)
						writeLine = func(v string) error {
							_, err := netInput.Write([]byte(v + "\n"))
							return err
						}
						defer netInput.Close()
					} else {
						log.Fatal("Cannot attach to Rumor, no endpoint was specified. Try --ipc, --tcp or --ws")
						return
					}

					stopped := false
					// Take the log of the net input, and feed it into our shell log
					go feedLog(log, nextLogLine, &stopped)

					// Take the input of our shell, and feed it to the net connection
					for {
						line, err := nextLine()
						if err != nil {
							if err != io.EOF {
								log.Error(err)
							}
							break
						}
						if err := writeLine(line); err != nil {
							log.Errorf("failed to send command: %v", err)
							break
						}
						if strings.TrimSpace(line) == "exit" {
							break
						}
					}
					stopped = true
				})
			},
		}

		attachCmd.Flags().StringVar(&ipcPath, "ipc", "", "Path of unix domain socket to attach to, e.g. `my_socket_file.sock`. Priority over `--tcp` flag.")
		attachCmd.Flags().StringVar(&tcpAddr, "tcp", "", "TCP socket address to attach to, e.g. `localhost:3030`.")
		attachCmd.Flags().StringVar(&wsAddr, "ws", "", "Http address (full URL) to attach to through websocket upgrade, e.g. `ws://localhost:8000/ws`. Disabled if empty.")
		attachCmd.Flags().StringVar(&wsApiKey, "ws-key", "", "Websocket API key ('X-Api-Key' header) to use for HTTP websocket upgrade requests. Not used if empty.")

		mainCmd.AddCommand(attachCmd)
	}
	{
		var wsAddr string
		var ipcPath string
		var tcpAddr string
		var wsApiKey string
		var wsPath string
		// TODO: maybe support websockets as well?

		serveCmd := &cobra.Command{
			Use:   "serve",
			Short: "Rumor as a server to attach to",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				log := logrus.New()
				log.SetOutput(os.Stdout)
				log.SetLevel(logrus.TraceLevel)
				log.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})

				var ipcInput net.Listener
				if ipcPath != "" {
					// clean up ipc file
					err := os.RemoveAll(ipcPath)
					if err != nil {
						log.Fatal("Cannot clean up old ipc path: ", err)
					}

					ipcInput, err = net.Listen("unix", ipcPath)
					if err != nil {
						log.Fatal("IPC listen error: ", err)
					}
				}
				var tcpInput net.Listener
				if tcpAddr != "" {
					var err error
					tcpInput, err = net.Listen("tcp", tcpAddr)
					if err != nil {
						log.Fatal("TCP listen error: ", err)
					}
				}

				sp := control.NewSessionProcessor(log)

				adminLog := log

				newSession := func(w io.Writer, r io.Reader, addr string) {
					log := logrus.New()
					log.SetOutput(w)
					log.SetLevel(logrus.TraceLevel)
					log.SetFormatter(&logrus.JSONFormatter{})

					adminLog.WithField("addr", addr).Info("new user session")

					sess := sp.NewSession(log)
					parser := syntax.NewParser()
					if err := parser.Interactive(r, func(stmts []*syntax.Stmt) bool {
						if parser.Incomplete() {
							return true
						}
						for _, stmt := range stmts {
							if err := sess.Run(context.Background(), stmt); err != nil {
								log.WithError(err).Error("error result")
								return true
							}
							if sess.Exited() {
								return false
							}
						}
						return true
					}); err != nil {
						log.WithError(err).Error("failed to run input")
					}
					<-sess.Done()

					adminLog.WithField("addr", addr).Info("user session stopped")

				}

				newConnSession := func(c net.Conn) {
					w := &TimedNetOut{c: c, timeout: UserWriteTimeout}
					r := &control.ReadWithCallback{
						Reader: c,
						Callback: func() error {
							// Reset read-deadline after every read
							return c.SetReadDeadline(time.Now().Add(UserReadTimeout))
						},
					}
					addr := c.RemoteAddr().String() + "/" + c.RemoteAddr().Network()
					newSession(w, r, addr)
					if err := c.Close(); err != nil {
						log.Error(err)
					}
				}

				newWsSession := func(c *websocket.Conn) {
					w := &TimedWsOut{c: c, timeout: UserWriteTimeout}

					timedUserInputLines := func() (s string, err error) {
						// Reset read-deadline.
						err = c.SetReadDeadline(time.Now().Add(UserReadTimeout))
						if err != nil {
							return "", err
						}
						mType, mContent, err := c.ReadMessage()
						if err != nil {
							return "", err
						}
						if mType != websocket.TextMessage {
							return "", errors.New("expected text-type websocket messages")
						}
						return string(mContent), nil
					}

					r := &control.LinesReader{Fn: timedUserInputLines}
					addr := c.RemoteAddr().String() + "/" + c.RemoteAddr().Network()

					newSession(w, r, addr)

					if err := c.Close(); err != nil {
						log.Error(err)
					}
				}

				stopped := false
				ctx, cancel := context.WithCancel(context.Background())
				sig := make(chan os.Signal, 1)
				signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
				go func() {
					sig := <-sig
					log.Printf("Caught signal %s: shutting down.", sig)
					stopped = true

					if ipcInput != nil {
						_ = ipcInput.Close()
					}
					if tcpInput != nil {
						_ = tcpInput.Close()
					}
					sp.Close()
					cancel()
					os.Exit(0)
				}()

				acceptInputs := func(input net.Listener) {
					for {
						// accept new connections and open a session for them
						c, err := input.Accept()
						if err != nil {
							// Don't error when we are shutting down, it is expected we cannot accept more connections
							if stopped {
								break
							}
							log.Error("Network connection accept error: ", err)
						}
						go newConnSession(c)
					}
				}
				if ipcInput != nil {
					go acceptInputs(ipcInput)
				}
				if tcpInput != nil {
					go acceptInputs(tcpInput)
				}

				if wsAddr != "" {
					http.Handle(wsPath, Middleware(
						http.HandlerFunc(makeWsHandler(log.WithField("ws-handler", wsAddr), newWsSession)),
						APIKeyCheck(func(key string) bool {
							return wsApiKey == "" || key == wsApiKey
						}).authMiddleware,
					))
					log.WithField("api-key", wsApiKey).Printf("listening for websocket upgrade requests on ws://%s%s", wsAddr, wsPath)
					log.Fatal(http.ListenAndServe(wsAddr, nil))
				}

				<-ctx.Done()
			},
		}

		serveCmd.Flags().StringVar(&ipcPath, "ipc", "", "Path to unix domain socket for IPC, e.g. `my_socket_file.sock`, use `rumor attach <socket>` to connect.")
		serveCmd.Flags().StringVar(&tcpAddr, "tcp", "", "TCP socket address to listen on, e.g. `localhost:3030`. Disabled if empty.")
		serveCmd.Flags().StringVar(&wsAddr, "ws", "", "Websocket address to listen on, e.g. `localhost:8000`. Disabled if empty.")
		serveCmd.Flags().StringVar(&wsPath, "ws-path", "/ws", "Path after websocket address to use for request upgrader.")
		serveCmd.Flags().StringVar(&wsApiKey, "ws-key", "", "Websocket API key ('X-Api-Key' header) to require from HTTP websocket upgrade requests. Not required if empty.")

		mainCmd.AddCommand(serveCmd)
	}

	{
		fileCmd := &cobra.Command{
			Use:   "file <input-file>",
			Short: "Run rom a file",
			Args:  cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				log := logrus.New()
				log.SetOutput(os.Stdout)
				log.SetLevel(logrus.TraceLevel)
				log.SetFormatter(&logrus.JSONFormatter{})

				inputFile, err := os.Open(args[0])
				if err != nil {
					log.Error(err)
					return
				}
				defer inputFile.Close()
				sp := control.NewSessionProcessor(log)
				sess := sp.NewSession(log)
				parser := syntax.NewParser()
				fileDoc, err := parser.Parse(inputFile, "")
				if err := sess.Run(context.Background(), fileDoc); err != nil {
					log.WithError(err).Error("failed to run script")
				}
				sess.Close()
				sp.Close()
			},
		}
		mainCmd.AddCommand(fileCmd)
	}

	{
		bareCmd := &cobra.Command{
			Use:   "bare",
			Short: "Rumor as a bare JSON-formatted input/output process, suitable for use as subprocess.",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				log := logrus.New()
				log.SetOutput(os.Stdout)
				log.SetLevel(logrus.TraceLevel)
				log.SetFormatter(&logrus.JSONFormatter{})

				r := io.Reader(os.Stdin)
				sp := control.NewSessionProcessor(log)
				sess := sp.NewSession(log)
				parser := syntax.NewParser()
				if err := parser.Interactive(r, func(stmts []*syntax.Stmt) bool {
					if parser.Incomplete() {
						return true
					}
					for _, stmt := range stmts {
						if err := sess.Run(context.Background(), stmt); err != nil {
							log.WithError(err).Error("error result")
							return true
						}
						if sess.Exited() {
							return false
						}
					}
					return true
				}); err != nil {
					log.WithError(err).Error("failed to run input")
				}
				sess.Close()
				sp.Close()
			},
		}
		mainCmd.AddCommand(bareCmd)
	}
	{
		var level string
		shellCmd := &cobra.Command{
			Use:   "shell",
			Short: "Rumor as a human-readable shell",
			Args: func(cmd *cobra.Command, args []string) error {
				if len(args) != 0 {
					return fmt.Errorf("interactive mode cannot process any arguments. Got: %s", strings.Join(args, " "))
				}
				return nil
			},
			Run: func(cmd *cobra.Command, args []string) {
				shellMode(level, func(log logrus.FieldLogger, nextLine control.NextMultiLineFn) {
					sp := control.NewSessionProcessor(log)
					sess := sp.NewSession(log)
					parser := syntax.NewParser()
					r := &control.LinesReader{Fn: nextLine}
					if err := parser.Interactive(r, func(stmts []*syntax.Stmt) bool {
						if parser.Incomplete() {
							fmt.Printf("incomplete! (%v)\n", stmts)
							return true
						}
						for _, stmt := range stmts {
							if err := sess.Run(context.Background(), stmt); err != nil {
								log.WithError(err).Error("error result")
								return true
							}
							if sess.Exited() {
								return false
							}
						}
						return true
					}); err != nil {
						if err != readline.ErrInterrupt {
							log.WithError(err).Error("failed to run input")
						}
					}
					sess.Close()
					sp.Close()
					// TODO exit codes
					os.Exit(0)
				})
			},
		}
		shellCmd.Flags().StringVar(&level, "level", "trace", "Log-level. Valid values: trace, debug, info, warn, error, fatal, panic")

		mainCmd.AddCommand(shellCmd)
	}

	if err := mainCmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to run Rumor: %v", err)
		os.Exit(1)
	} else {
		os.Exit(0)
	}
}

func Middleware(h http.Handler, middleware ...func(http.Handler) http.Handler) http.Handler {
	for _, mw := range middleware {
		h = mw(h)
	}
	return h
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func makeWsHandler(log logrus.FieldLogger, handleWs func(conn *websocket.Conn)) func(rw http.ResponseWriter, req *http.Request) {
	return func(rw http.ResponseWriter, req *http.Request) {
		wsConn, err := upgrader.Upgrade(rw, req, nil)
		if err != nil {
			log.Printf("upgrade err: %v", err)
			return
		}
		defer wsConn.Close()

		handleWs(wsConn)
	}
}

type APIKeyCheck func(key string) bool

func (kc APIKeyCheck) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		apiKey := req.Header.Get("X-Api-Key")
		if !kc(apiKey) {
			rw.WriteHeader(http.StatusForbidden)
		} else {
			next.ServeHTTP(rw, req)
		}
	})
}
