package main

import (
	"fmt"
	"github.com/protolambda/rumor/sh"
	"github.com/spf13/cobra"
	"os"
)

func main() {

	mainCmd := cobra.Command{
		Use:   "rumor",
		Short: "Start Rumor",
	}
	mainCmd.AddCommand(sh.AttachCmd(), sh.BareCmd(), sh.FileCmd(), sh.ServeCmd(), sh.ShellCmd(), sh.ToolCmd())

	if err := mainCmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to run Rumor: %v", err)
		os.Exit(1)
	} else {
		os.Exit(0)
	}
}
