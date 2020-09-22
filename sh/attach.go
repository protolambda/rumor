package sh

import (
	"errors"
	"fmt"
	"github.com/chzyer/readline"
	"github.com/gorilla/websocket"
	"github.com/protolambda/rumor/control"
	"github.com/spf13/cobra"
	"io"
	"mvdan.cc/sh/v3/syntax"
	"net"
	"net/http"
	"net/url"
	"os"
)

func AttachCmd() *cobra.Command {
	var wsAddr string
	var ipcPath string
	var tcpAddr string
	var wsApiKey string

	var level string
	cmd := &cobra.Command{
		Use:   "attach",
		Short: "Attach to a running rumor server",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			rl, err := shellReader()
			if err != nil {
				fmt.Errorf("could not open shell reader: %v", err)
				os.Exit(1)
				return
			}
			log, err := shellLogger(level, rl.Operation.Stdout())
			if err != nil {
				log.WithError(err).Error("could not open shell logger")
				os.Exit(1)
				return
			}
			shellMode(rl, func(nextLine control.NextMultiLineFn, onParse ParseBufferFn) {
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
				// Use a local parser just to get the input consumption right.
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
	cmd.Flags().StringVar(&level, "level", "debug", "Log-level of the attached client. Valid values: trace, debug, info, warn, error, fatal, panic")
	cmd.Flags().StringVar(&ipcPath, "ipc", "", "Path of unix domain socket to attach to, e.g. `my_socket_file.sock`. Priority over `--tcp` flag.")
	cmd.Flags().StringVar(&tcpAddr, "tcp", "", "TCP socket address to attach to, e.g. `localhost:3030`.")
	cmd.Flags().StringVar(&wsAddr, "ws", "", "Http address (full URL) to attach to through websocket upgrade, e.g. `ws://localhost:8000/ws`. Disabled if empty.")
	cmd.Flags().StringVar(&wsApiKey, "ws-key", "", "Websocket API key ('X-Api-Key' header) to use for HTTP websocket upgrade requests. Not used if empty.")

	return cmd
}
