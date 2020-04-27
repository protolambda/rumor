package main

import (
	"bufio"
	"fmt"
	"github.com/chzyer/readline"
	"github.com/protolambda/rumor/mngmt"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"net"
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
	fromFilepath := ""

	mainCmd := cobra.Command{
		Use:   "rumor",
		Short: "Start Rumor",
	}

	asLineReader := func(r io.Reader) func() (s string, err error) {
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
		attachCmd := &cobra.Command{
			Use:   "attach",
			Short: "Attach to a running rumor server",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				// TODO
			},
		}
		mainCmd.AddCommand(attachCmd)
	}
	{
		var ipcPath string
		var tcpAddr string
		// TODO: websockets

		serveCmd := &cobra.Command{
			Use:   "serve",
			Short: "Rumor as a server to attach to",
			Args: cobra.NoArgs,
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

				sp := mngmt.NewSessionProcessor(log)

				newSession := func(c net.Conn) {
					log := logrus.New()
					log.SetOutput(c)
					log.SetLevel(logrus.TraceLevel)
					log.SetFormatter(&logrus.JSONFormatter{})

					<-sp.NewSession(log, asLineReader(c)).Done()
				}

				sig := make(chan os.Signal, 1)
				signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
				go func() {
					sig := <-sig
					log.Printf("Caught signal %s: shutting down.", sig)
					if ipcInput != nil {
						_ = ipcInput.Close()
					}
					if tcpInput != nil {
						_ = tcpInput.Close()
					}
					sp.Close()
					os.Exit(0)
				}()

				acceptInputs := func(input net.Listener) {
					for {
						// accept new connections and open a session for them
						fd, err := input.Accept()
						if err != nil {
							log.Error("Accept error: ", err)
							continue
						}

						go newSession(fd)
					}
				}
				if ipcInput != nil {
					acceptInputs(ipcInput)
				}
				if tcpInput != nil {
					acceptInputs(tcpInput)
				}
			},
		}

		serveCmd.Flags().StringVar(&ipcPath, "ipc", "", "Path to unix domain socket for IPC, e.g. `my_socket_file.sock`, use `rumor attach <socket>` to connect.")
		serveCmd.Flags().StringVar(&tcpAddr, "tcp", "", "TCP socket address to listen on, e.g. `localhost:3030`. Disabled if empty.")

		mainCmd.AddCommand(serveCmd)
	}

	{
		bareCmd := &cobra.Command{
			Use:   "bare [input-file]",
			Short: "Rumor as a bare JSON-formatted input/output process, suitable for use as subprocess. Optionally read input from a file instead of stdin.",
			Args: func(cmd *cobra.Command, args []string) error {
				if len(args) > 1 {
					return fmt.Errorf("non-interactive mode cannot have more than 1 argument. Got: %s", strings.Join(args, " "))
				}
				if len(args) == 1 {
					fromFilepath = args[0]
				}
				return nil
			},
			Run: func(cmd *cobra.Command, args []string) {
				log := logrus.New()
				log.SetOutput(os.Stdout)
				log.SetLevel(logrus.TraceLevel)
				log.SetFormatter(&logrus.JSONFormatter{})

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
				nextLine := asLineReader(r)
				sp := mngmt.NewSessionProcessor(log)
				<-sp.NewSession(log, nextLine).Done()
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

				if level != "" {
					logLevel, err := logrus.ParseLevel(level)
					if err != nil {
						log.Error(err)
					}
					log.SetLevel(logLevel)
				}

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

				sp := mngmt.NewSessionProcessor(log)
				<-sp.NewSession(log, l.Readline).Done()
				sp.Close()
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
