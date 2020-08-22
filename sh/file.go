package sh

import (
	"context"
	"github.com/protolambda/rumor/control"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
	"os"
	"time"
)

func FileCmd() *cobra.Command {
	var level string
	var formatter string
	cmd := &cobra.Command{
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
	cmd.Flags().StringVar(&level, "level", "debug", "Log-level. Valid values: trace, debug, info, warn, error, fatal, panic")
	cmd.Flags().StringVar(&formatter, "formatter", "json", "Formatter. Valid values: json, terminal")
	return cmd
}
