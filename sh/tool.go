package sh

import (
	"context"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/tool"
	"github.com/spf13/cobra"
	"os"
)

func ToolCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "tool",
		Short:              "Run rumor toolchain",
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true, // pass flags as args, need to pass to Ask CLI library underneath.
		Run: func(cmd *cobra.Command, args []string) {
			callCmd := &tool.ToolCmd{
				Out: cmd.OutOrStdout(),
			}
			loadedCmd, err := ask.Load(callCmd)
			if err != nil {
				cmd.PrintErr(err)
				os.Exit(1)
				return
			}
			fCmd, isHelp, err := loadedCmd.Execute(context.Background(), args...)
			if err != nil {
				cmd.PrintErr(err)
				os.Exit(1)
			} else {
				if isHelp {
					cmd.PrintErr(fCmd.Usage())
					os.Exit(2)
				}
				os.Exit(0)
			}
		},
	}
}
