package chcmd

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
)

type VotesCmd struct {
	*base.Base
	// TODO: ask command parsing: support []uint64
	//Indices []uint64 `ask:"--indices" help:"Only fetch votes for a subset of validators"`
}

func (c *VotesCmd) Help() string {
	return "List the latest votes of validators"
}

func (c *VotesCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
