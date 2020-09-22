package sh

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/chzyer/readline"
	"github.com/gliderlabs/ssh"
	"github.com/gorilla/websocket"
	"github.com/protolambda/rumor/control"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

type Server struct {
	log      logrus.FieldLogger
	sp       *control.SessionProcessor
	sshUsers map[string]string
	stopped  bool
}

func (s *Server) ipc(path string) (close func() error, err error) {
	// clean up ipc file
	if err := os.RemoveAll(path); err != nil {
		return nil, fmt.Errorf("cannot clean up old ipc path: %v", err)
	}

	ipcInput, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("IPC listen error: %v", err)
	}
	go s.acceptInputs(ipcInput)
	return ipcInput.Close, nil
}

func (s *Server) tcp(addr string) (close func() error, err error) {
	tcpInput, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("TCP listen error: %v", err)
	}
	go s.acceptInputs(tcpInput)
	return tcpInput.Close, nil
}

func (s *Server) newConnSession(c net.Conn) {
	w := &TimedNetOut{c: c, timeout: UserWriteTimeout}
	r := &control.ReadWithCallback{
		Reader: c,
		Callback: func() error {
			// Reset read-deadline after every read
			return c.SetReadDeadline(time.Now().Add(UserReadTimeout))
		},
	}
	addr := c.RemoteAddr().String() + "/" + c.RemoteAddr().Network()
	s.newSession(w, r, addr)
	if err := c.Close(); err != nil {
		s.log.Error(err)
	}
}

func (s *Server) newSession(w io.Writer, r io.Reader, addr string) {
	log := logrus.New()
	log.SetOutput(w)
	log.SetLevel(logrus.TraceLevel)
	log.SetFormatter(&logrus.JSONFormatter{TimestampFormat: time.RFC3339Nano})

	s.log.WithField("addr", addr).Info("new user session")

	sess := s.sp.NewSession(log)
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

	s.log.WithField("addr", addr).Info("user session stopped")
}

func parseAuthorizedKeys(authorizedKeysFile string) ([]ssh.PublicKey, error) {
	file, err := os.Open(authorizedKeysFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	keys := []ssh.PublicKey{}
	for scanner.Scan() {
		keyData := scanner.Bytes()
		if len(keyData) == 0 {
			// whitespace in authorized keys file... skip
			continue
		}
		key, _, _, _, err := ssh.ParseAuthorizedKey(keyData)
		if err != nil {
			return keys, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func (s *Server) ssh(sshAddr string, sshHostKeyFile string, authorizedKeysFile string) (close func() error, err error) {
	var authorizedKeys []ssh.PublicKey
	if authorizedKeysFile != "" {
		var err error
		authorizedKeys, err = parseAuthorizedKeys(authorizedKeysFile)
		if err != nil {
			s.log.Error(err)
			os.Exit(1)
		}
	}
	sshServ := &ssh.Server{
		Addr: sshAddr,
		Handler: func(session ssh.Session) {
			_, _ = io.WriteString(session, "Opening new rumor session...")
			userV := session.Context().Value(ssh.ContextKeyUser)
			sessionID := session.Context().Value(ssh.ContextKeySessionID)
			addr := session.Context().Value(ssh.ContextKeyRemoteAddr).(net.Addr).String()
			userData := logrus.Fields{"addr": addr, "id": sessionID}
			if user, ok := userV.(string); ok {
				userData["user"] = user
			} else {
				pub, ok := session.Context().Value(ssh.ContextKeyPublicKey).(ssh.PublicKey)
				if ok {
					userData["pub"] = hex.EncodeToString(pub.Marshal())
				} else {
					s.log.Warn("unrecognized ssh login method")
					_ = session.Exit(1)
					return
				}
			}
			s.log.WithFields(userData).Info("ssh session opened")
			if _, _, ok := session.Pty(); ok {
				s.handleSshPtyUser(session)
			} else {
				// TODO: support pure stdin/out?
				s.log.Warn("SSH session without PTY request")
			}
			_ = session.Close()
			s.log.WithFields(userData).Info("ssh session ended")
		},
		PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) bool {
			for _, authorizedKey := range authorizedKeys {
				if ssh.KeysEqual(key, authorizedKey) {
					return true
				}
			}
			return false
		},
		PasswordHandler: func(ctx ssh.Context, pass string) bool {
			user := ctx.Value(ssh.ContextKeyUser).(string)
			addr := ctx.Value(ssh.ContextKeyRemoteAddr).(net.Addr).String()
			expected, ok := s.sshUsers[user]
			if ok && pass == expected {
				return true
			} else {
				s.log.WithFields(logrus.Fields{"user": user, "addr": addr}).Warn("bad ssh user login attempt")
				return false
			}
		},
	}
	if sshHostKeyFile != "" {
		if err := ssh.HostKeyFile(sshHostKeyFile)(sshServ); err != nil {
			s.log.Error(err)
			os.Exit(1)
		}
	}
	go sshServ.ListenAndServe()
	return sshServ.Close, nil
}

func (s *Server) handleSshPtyUser(session ssh.Session) {
	rl, _ := readline.NewEx(&readline.Config{
		Prompt:              startPrompt,
		HistoryFile:         "/tmp/rumor-history.tmp", // TODO different history per user
		InterruptPrompt:     "^C",
		EOFPrompt:           "exit",
		HistorySearchFold:   true,
		FuncFilterInputRune: filterInput,
		Stdin:               session,
		Stdout:              session,
		Stderr:              session.Stderr(),
		FuncGetWidth: func() int {
			r, _, _ := session.Pty()
			return r.Window.Width
		},
		FuncIsTerminal: func() bool {
			_, _, ok := session.Pty()
			return ok
		},
		FuncOnWidthChanged: func(f func()) {
			_, wch, _ := session.Pty()
			go func() {
				for {
					_, ok := <-wch
					if !ok {
						break
					}
					f()
				}
			}()
		},
		DisableAutoSaveHistory: true, // disabled to deal with multi-line history better
	})

	log := logrus.New()
	rlw := rl.Operation.Stdout() // allocate the writer once, avoid rl.Write directly.
	log.SetOutput(rlw)
	log.SetLevel(logrus.DebugLevel)
	log.SetFormatter(&control.ShellLogFmt{})

	shellMode(rl,
		func(nextLine control.NextMultiLineFn, onParse ParseBufferFn) {
			sess := s.sp.NewSession(log)
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
		})
}

func (s *Server) http(httpAddr, apiKey string) (close func() error, err error) {
	m := http.NewServeMux()

	m.Handle("/ws", Middleware(
		http.HandlerFunc(makeWsHandler(s.log.WithField("ws-handler", httpAddr), s.newWsSession)),
		apiKeyMiddleware(apiKey),
	))

	m.Handle("/script", Middleware(
		http.HandlerFunc(s.newHttpPost),
		apiKeyMiddleware(apiKey),
	))

	m.Handle("/report", Middleware(
		http.HandlerFunc(s.newHttpReport),
		apiKeyMiddleware(apiKey),
	))

	s.log.WithField("http", "http://"+httpAddr).Info("listening for report requests")

	if apiKey != "" {
		s.log.WithField("api-key", apiKey).Info("Requiring simple auth, use 'X-Api-Key' header")
	}
	srv := &http.Server{
		Addr:         httpAddr,
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      m,
	}
	go srv.ListenAndServe()
	return srv.Close, nil
}

func (s *Server) newWsSession(c *websocket.Conn) {
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

	s.newSession(w, r, addr)

	if err := c.Close(); err != nil {
		s.log.Error(err)
	}
}

func (s *Server) newHttpPost(rw http.ResponseWriter, req *http.Request) {
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

	sess := s.sp.NewSession(log)
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
		s.log.Warn("failed to close session: %v", sess)
	}
}

func (s *Server) newHttpReport(rw http.ResponseWriter, req *http.Request) {
	rw.Header().Set("Content-Type", "application/json; charset=UTF-8")
	enc := json.NewEncoder(rw)
	if err := enc.Encode(s.sp.Report()); err != nil {
		s.log.WithError(err).Warn("failed to respond to HTTP report request")
	}
}

func (s *Server) acceptInputs(input net.Listener) {
	for {
		// accept new connections and open a session for them
		c, err := input.Accept()
		if err != nil {
			s.log.Warn("Stopped accepting connections: ", err)
			return
		}
		go s.newConnSession(c)
	}
}

func ServeCmd() *cobra.Command {
	var level string
	var httpAddr string
	var ipcPath string
	var tcpAddr string
	var apiKey string
	var sshAddr string
	var sshUsers []string
	var sshHostKeyFile string
	var sshAuthorizedKeysFile string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Rumor as a server to attach to",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			log := logrus.New()
			log.SetOutput(os.Stdout)
			log.SetLevel(logrus.DebugLevel)
			log.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})

			if level != "" {
				logLevel, err := logrus.ParseLevel(level)
				if err != nil {
					log.Error(err)
				}
				log.SetLevel(logLevel)
			}
			s := &Server{log: log, sp: control.NewSessionProcessor(log)}

			if sshAddr != "" {
				passwords := make(map[string]string, len(sshUsers))
				for _, u := range sshUsers {
					dat := strings.Split(u, ":")
					if len(dat) != 2 {
						log.Warn("invalid ssh password entry, ignoring")
						continue
					}
					passwords[dat[0]] = dat[1]
				}
				s.sshUsers = passwords
			}

			ctx, cancel := context.WithCancel(context.Background())
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
			go func() {
				sig := <-sig
				log.Printf("Caught signal %s: shutting down.", sig)
				s.stopped = true
				s.sp.Close()
				cancel()
				os.Exit(0)
			}()

			closeAll := func() {
				log.Info("done")
			}
			serving := func(close func() error, err error) {
				if err != nil {
					log.WithError(err).Info("cannot start listener")
				}
				closeAll = func() {
					if err := close(); err != nil {
						log.WithError(err).Error("failed to close listener")
					}
				}
			}
			if ipcPath != "" {
				serving(s.ipc(ipcPath))
			}
			if tcpAddr != "" {
				serving(s.tcp(tcpAddr))
			}
			if sshAddr != "" {
				serving(s.ssh(sshAddr, sshHostKeyFile, sshAuthorizedKeysFile))
			}
			if httpAddr != "" {
				serving(s.http(httpAddr, apiKey))
			}

			log.Info("Started server")

			<-ctx.Done()
		},
	}
	cmd.Flags().StringVar(&level, "level", "debug", "Log-level of the server log. Valid values: trace, debug, info, warn, error, fatal, panic")
	cmd.Flags().StringVar(&ipcPath, "ipc", "", "Path to unix domain socket for IPC, e.g. 'my_socket_file.sock', use 'rumor attach <socket>' to connect.")
	cmd.Flags().StringVar(&tcpAddr, "tcp", "", "TCP socket address to listen on, e.g. 'localhost:3030'. Disabled if empty.")
	cmd.Flags().StringVar(&httpAddr, "http", "", "Websocket/HTTP address to listen on, e.g. 'localhost:8000'. Disabled if empty.")
	cmd.Flags().StringVar(&sshAddr, "ssh", "", "SSH address to listen on, e.g. '127.0.0.1:5000'. Disabled if empty")
	cmd.Flags().StringVar(&sshHostKeyFile, "ssh-key", "", "SSH host key. Temporary key generated randomly if empty.")
	cmd.Flags().StringArrayVar(&sshUsers, "ssh-users", []string{}, "Super simple SSH users. Formatted as 'user:pass'")
	cmd.Flags().StringVar(&sshAuthorizedKeysFile, "ssh-authorized-keys", "", "Path to `authorized_keys` file, as used in OpenSSH")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "Websocket/HTTP API key ('X-Api-Key' header) to require from HTTP requests and websocket upgrade requests. No key required if empty.")
	return cmd
}

func apiKeyMiddleware(apiKey string) func(http.Handler) http.Handler {
	return APIKeyCheck(func(key string) bool {
		return apiKey == "" || key == apiKey
	}).authMiddleware
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
