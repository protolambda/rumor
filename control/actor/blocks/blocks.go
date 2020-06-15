package blocks

import (
	"context"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"strconv"
	"time"
)

type BlocksState struct {

}

func (r *Actor) InitBlocksCmd(ctx context.Context, log logrus.FieldLogger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "blocks",
		Short: "Manage Eth2 block database",
	}

	/*
	 TODO blocks cmd

	    import
	    stats
	    get
	    set
	 */
	cmd.AddCommand(&cobra.Command{
		Use:   "sleep <ms>",
		Short: "Sleep for given amount of milliseconds",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			count, err := strconv.ParseUint(args[0], 0, 64)
			if err != nil {
				log.Error(err)
				return
			}
			log.Infoln("started sleeping!")
			sleepCtx, _ := context.WithTimeout(ctx, time.Duration(count)*time.Millisecond)
			<-sleepCtx.Done()
			if ctx.Err() == nil {
				log.Infoln("done sleeping!")
			} else {
				log.Infof("stopped sleep, exit early: %v", ctx.Err())
			}
		},
	})
	return cmd
}
