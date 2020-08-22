package sh

import (
	"context"
	"github.com/protolambda/rumor/control"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
	"os"
	"time"
)

func BareCmd() *cobra.Command {
	var level string
	var stopOnErr bool
	cmd := &cobra.Command{
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
	cmd.Flags().StringVar(&level, "level", "debug", "Log-level. Valid values: trace, debug, info, warn, error, fatal, panic")
	cmd.Flags().BoolVar(&stopOnErr, "stop-on-err", false, "When a command exits with an error, stop processing input.")

	return cmd
}
