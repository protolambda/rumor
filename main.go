package main

import (
	"bufio"
	"bytes"
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
	"mvdan.cc/sh/v3/interp"
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

func main() {

	mainCmd := cobra.Command{
		Use:   "rumor",
		Short: "Start Rumor",
	}

	// multi-line inputs end each intermediate line with "\"
	shellMode := func(levelFlag string, run func(log logrus.FieldLogger, nextLine control.NextMultiLineFn, onParse ParseBufferFn)) {
		log := logrus.New()
		log.SetOutput(os.Stdout)
		log.SetLevel(logrus.TraceLevel)
		log.SetFormatter(&control.ShellLogFmt{})

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
		run(log, readMultiLine, onParse)
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
			delete(entry, "time")
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
				shellMode(level, func(log logrus.FieldLogger, nextLine control.NextMultiLineFn, onParse ParseBufferFn) {
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
					// Use a locla parser just to get the input consumption right.
					// Auto-detect things like incomplete commands due to line breaks.
					localParser := syntax.NewParser()
					r := &control.LinesReader{Fn: nextLine}
					for {
						if err := localParser.Interactive(r, func(stmts []*syntax.Stmt) bool {
							if localParser.Incomplete() {
								onParse(false, false)
								return true
							}
							fullyParsed := onParse(true, false)

							if err := writeLine(fullyParsed); err != nil {
								log.Errorf("failed to send command: %v", err)
							}
							return true
						}); err != nil {
							// keep the shell alive after hitting bad inputs,
							// we don't want a syntax mistake to tear down a session.
							if err != readline.ErrInterrupt && err != io.EOF {
								onParse(false, true)
								log.Error(err)
								continue
							} else {
								break
							}
						}
					}
					stopped = true
				})
			},
		}
		attachCmd.Flags().StringVar(&level, "level", "debug", "Log-level of the attached client. Valid values: trace, debug, info, warn, error, fatal, panic")
		attachCmd.Flags().StringVar(&ipcPath, "ipc", "", "Path of unix domain socket to attach to, e.g. `my_socket_file.sock`. Priority over `--tcp` flag.")
		attachCmd.Flags().StringVar(&tcpAddr, "tcp", "", "TCP socket address to attach to, e.g. `localhost:3030`.")
		attachCmd.Flags().StringVar(&wsAddr, "ws", "", "Http address (full URL) to attach to through websocket upgrade, e.g. `ws://localhost:8000/ws`. Disabled if empty.")
		attachCmd.Flags().StringVar(&wsApiKey, "ws-key", "", "Websocket API key ('X-Api-Key' header) to use for HTTP websocket upgrade requests. Not used if empty.")

		mainCmd.AddCommand(attachCmd)
	}
	{
		var level string
		var httpAddr string
		var ipcPath string
		var tcpAddr string
		var apiKey string
		var wsPath string
		var postPath string
		var reportToURL string

		serveCmd := &cobra.Command{
			Use:   "serve",
			Short: "Rumor as a server to attach to",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				log := logrus.New()
				log.SetOutput(os.Stdout)
				log.SetLevel(logrus.TraceLevel)
				log.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})

				if level != "" {
					logLevel, err := logrus.ParseLevel(level)
					if err != nil {
						log.Error(err)
					}
					log.SetLevel(logLevel)
				}

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
				if reportToURL != "" {

				}

				adminLog := log

				newSession := func(w io.Writer, r io.Reader, addr string) {
					log := logrus.New()
					log.SetOutput(w)
					log.SetLevel(logrus.TraceLevel)
					log.SetFormatter(&logrus.JSONFormatter{TimestampFormat: time.RFC3339Nano})

					adminLog.WithField("addr", addr).Info("new user session")

					sess := sp.NewSession(log)
					parser := syntax.NewParser()
					if err := parser.Interactive(r, func(stmts []*syntax.Stmt) bool {
						if parser.Incomplete() {
							return true
						}
						for _, stmt := range stmts {
							if err := sess.Run(context.Background(), stmt); err != nil {
								if e, ok := interp.IsExitStatus(err); ok {
									log.WithField("code", e).Error("non-zero exit")
								} else {
									log.WithError(err).Error("error result")
								}
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
						return string(mContent) + "\n", nil
					}

					r := &control.LinesReader{Fn: timedUserInputLines}
					addr := c.RemoteAddr().String() + "/" + c.RemoteAddr().Network()

					newSession(w, r, addr)

					if err := c.Close(); err != nil {
						log.Error(err)
					}
				}

				newHttpPost := func(rw http.ResponseWriter, req *http.Request) {
					log := logrus.New()
					var buf bytes.Buffer
					log.SetOutput(&buf)

					lvl, err := logrus.ParseLevel(req.Header.Get("X-Log-Level"))
					if err != nil {
						lvl = logrus.DebugLevel
					}
					log.SetLevel(lvl)
					if req.Header.Get("X-Log-Format") == "terminal" {
						log.SetFormatter(&control.ShellLogFmt{})
					} else {
						log.SetFormatter(&logrus.JSONFormatter{TimestampFormat: time.RFC3339Nano})
					}

					sess := sp.NewSession(log)
					parser := syntax.NewParser()
					fileDoc, err := parser.Parse(req.Body, "")
					if err != nil {
						rw.WriteHeader(http.StatusBadRequest)
						_, _ = rw.Write([]byte(err.Error()))
					}
					_ = req.Body.Close()
					if err := sess.Run(context.Background(), fileDoc); err != nil {
						if e, ok := interp.IsExitStatus(err); ok {
							switch e {
							case 0:
								rw.WriteHeader(http.StatusOK)
							case 1:
								rw.WriteHeader(http.StatusInternalServerError)
							case 2:
								rw.WriteHeader(http.StatusBadRequest)
							default:
								rw.WriteHeader(http.StatusTeapot)
							}
						} else if err != nil {
							log.WithError(err).Error("error result")
							rw.WriteHeader(http.StatusInternalServerError)
						}
					}
					_, _ = rw.Write(buf.Bytes())
					if err := sess.Close(); err != nil {
						adminLog.Warn("failed to close session: %v", sess)
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

				if httpAddr != "" {
					http.Handle(wsPath, Middleware(
						http.HandlerFunc(makeWsHandler(log.WithField("ws-handler", httpAddr), newWsSession)),
						APIKeyCheck(func(key string) bool {
							return apiKey == "" || key == apiKey
						}).authMiddleware,
					))
					http.Handle(postPath, Middleware(
						http.HandlerFunc(newHttpPost),
						APIKeyCheck(func(key string) bool {
							return apiKey == "" || key == apiKey
						}).authMiddleware,
					))
					log.WithFields(logrus.Fields{"api-key": apiKey,
						"ws":   fmt.Sprintf("ws://%s%s", httpAddr, wsPath),
						"post": fmt.Sprintf("http://%s%s", httpAddr, postPath),
					}).Print("listening for websocket upgrade requests and script posts")
					log.Fatal(http.ListenAndServe(httpAddr, nil))
				}

				adminLog.Info("Started server")

				<-ctx.Done()
			},
		}
		serveCmd.Flags().StringVar(&level, "level", "debug", "Log-level of the server log. Valid values: trace, debug, info, warn, error, fatal, panic")
		serveCmd.Flags().StringVar(&ipcPath, "ipc", "", "Path to unix domain socket for IPC, e.g. `my_socket_file.sock`, use `rumor attach <socket>` to connect.")
		serveCmd.Flags().StringVar(&tcpAddr, "tcp", "", "TCP socket address to listen on, e.g. `localhost:3030`. Disabled if empty.")
		serveCmd.Flags().StringVar(&httpAddr, "http", "", "Websocket/HTTP address to listen on, e.g. `localhost:8000`. Disabled if empty.")
		serveCmd.Flags().StringVar(&wsPath, "ws-path", "/ws", "Path after http address to use for request upgrader.")
		serveCmd.Flags().StringVar(&postPath, "post-path", "/script", "Path after http address to use for executing http-POST scripts")
		serveCmd.Flags().StringVar(&apiKey, "api-key", "", "Websocket/HTTP API key ('X-Api-Key' header) to require from HTTP requests and websocket upgrade requests. Not required if empty.")

		mainCmd.AddCommand(serveCmd)
	}

	{
		var level string
		var formatter string
		fileCmd := &cobra.Command{
			Use:   "file <input-file>",
			Short: "Run from a file",
			Args:  cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				log := logrus.New()
				log.SetOutput(os.Stdout)
				log.SetLevel(logrus.TraceLevel)
				if formatter == "terminal" {
					log.SetFormatter(&control.ShellLogFmt{})
				} else {
					log.SetFormatter(&logrus.JSONFormatter{TimestampFormat: time.RFC3339Nano})
				}

				if level != "" {
					logLevel, err := logrus.ParseLevel(level)
					if err != nil {
						log.Error(err)
					}
					log.SetLevel(logLevel)
				}

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
				if err != nil {
					log.Error(err)
					return
				}
				exitCode := uint8(0)
				if err := sess.Run(context.Background(), fileDoc); err != nil {
					if e, ok := interp.IsExitStatus(err); ok {
						exitCode = e
					} else if err != nil {
						log.WithError(err).Error("error result")
						exitCode = 1
					}
				}
				sess.Close()
				sp.Close()
				os.Exit(int(exitCode))
			},
		}
		fileCmd.Flags().StringVar(&level, "level", "debug", "Log-level. Valid values: trace, debug, info, warn, error, fatal, panic")
		fileCmd.Flags().StringVar(&formatter, "formatter", "json", "Formatter. Valid values: json, terminal")
		mainCmd.AddCommand(fileCmd)
	}

	{
		var level string
		var stopOnErr bool
		bareCmd := &cobra.Command{
			Use:   "bare",
			Short: "Rumor as a bare JSON-formatted input/output process, suitable for use as subprocess.",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				log := logrus.New()
				log.SetOutput(os.Stdout)
				log.SetLevel(logrus.TraceLevel)
				log.SetFormatter(&logrus.JSONFormatter{TimestampFormat: time.RFC3339Nano})

				if level != "" {
					logLevel, err := logrus.ParseLevel(level)
					if err != nil {
						log.Error(err)
					}
					log.SetLevel(logLevel)
				}

				r := io.Reader(os.Stdin)
				sp := control.NewSessionProcessor(log)
				sess := sp.NewSession(log)
				parser := syntax.NewParser()
				exitCode := uint8(0)
				if err := parser.Interactive(r, func(stmts []*syntax.Stmt) bool {
					if parser.Incomplete() {
						return true
					}
					for _, stmt := range stmts {
						if err := sess.Run(context.Background(), stmt); err != nil {
							if stopOnErr {
								if e, ok := interp.IsExitStatus(err); ok {
									exitCode = e
									return false
								}
								if err != nil {
									log.WithError(err).Error("error result")
									exitCode = 1
									return false
								}
							}
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
				os.Exit(int(exitCode))
			},
		}
		bareCmd.Flags().StringVar(&level, "level", "debug", "Log-level. Valid values: trace, debug, info, warn, error, fatal, panic")
		bareCmd.Flags().BoolVar(&stopOnErr, "stop-on-err", false, "When a command exits with an error, stop processing input.")

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
				shellMode(level, func(log logrus.FieldLogger, nextLine control.NextMultiLineFn, onParse ParseBufferFn) {
					sp := control.NewSessionProcessor(log)
					sess := sp.NewSession(log)
					parser := syntax.NewParser()
					r := &control.LinesReader{Fn: nextLine}
					for {
						if err := parser.Interactive(r, func(stmts []*syntax.Stmt) bool {
							if parser.Incomplete() {
								onParse(false, false)
								return true
							}
							fullyParsed := onParse(true, false)
							log.Debug(fullyParsed)
							for _, stmt := range stmts {
								if err := sess.Run(context.Background(), stmt); err != nil {
									if e, ok := interp.IsExitStatus(err); ok {
										log.WithField("code", e).Error("non-zero exit")
									} else {
										log.WithError(err).Error("error result")
									}
									return true
								}
								if sess.Exited() {
									return false
								}
							}
							return true
						}); err != nil {
							// keep the shell alive after hitting bad inputs,
							// we don't want a syntax mistake to tear down a session.
							if err != readline.ErrInterrupt && !sess.Exited() {
								onParse(false, true)
								log.Error(err)
								continue
							} else {
								break
							}
						}
						if sess.Exited() {
							break
						}
					}
					sess.Close()
					sp.Close()
					os.Exit(0)
				})
			},
		}
		shellCmd.Flags().StringVar(&level, "level", "debug", "Log-level. Valid values: trace, debug, info, warn, error, fatal, panic")

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
