package sh

import (
	"context"
	"fmt"
	"github.com/chzyer/readline"
	"github.com/protolambda/rumor/control"
	"github.com/spf13/cobra"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
	"os"
	"strings"
)

func ShellCmd() *cobra.Command {
	var level string
	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Rumor as a human-readable shell",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("interactive mode cannot process any arguments. Got: %s", strings.Join(args, " "))
			}
			return nil
		},
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
						onParse(true, false)
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
	cmd.Flags().StringVar(&level, "level", "debug", "Log-level. Valid values: trace, debug, info, warn, error, fatal, panic")
	return cmd
}
