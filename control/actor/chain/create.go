package chain

/*
TODO
	create <chain name> <genesis>
*/

import (
	"context"
	"github.com/protolambda/rumor/chain"
	"github.com/protolambda/rumor/control/actor/base"
)

type ChainCreateCmd struct {
	*base.Base
	Name       chain.ChainID `ask:"<name>" help:"The name to give to the created chain. Must not exist yet."`
	StateInput string        `ask:"--state-file" help:"State input file"`
	StateData  []byte        `ask:"--state-data" help:"Alternative to state input, read state from bytes directly"`
}

func (c *ChainCreateCmd) Help() string {
	return "Create a new eth2 chain from a pre-state"
}

func (c *ChainCreateCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
