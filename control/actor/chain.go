package actor

import (
	"context"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type ChainState struct {

}

func (r *Actor) InitChainCmd(ctx context.Context, log logrus.FieldLogger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chain",
		Short: "Manage Eth2 chains, forkchoice too",
	}

	/*
	 TODO chain cmd

	    switch <chain name>  # Change actor to a different chain
	    create <chain name> <genesis>
	    copy <chain name>  # fork the chain by creating a copy of it
	    hot
	      view [anchor root]
	    cold
	      view <start> <stop>
	    block
	      import <root>  # import block (reference to block db)

	    attestation
	      import   # import attestation
	      gossip   # track gossip to import attestations
	        subnet <index>
	        global

	    votes ...     # get current latest votes

	    head  # Manage current head of current chain
	      get
	      set <block root>    # fixed point, don't change status otherways
	      follow  # follow proto-array forkchoice of chain

	    serve  # Serve the current chain
	      by-range
	      by-root
	 */
	cmd.AddCommand(&cobra.Command{
		Use:   "switch <chainname>",
		Short: "Switch to an existing chain",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			// TODO
		},
	})
	return cmd
}
