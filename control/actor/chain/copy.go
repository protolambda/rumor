package chain

import (
	"context"
	"errors"
	"github.com/protolambda/rumor/chain"
	"github.com/protolambda/rumor/control/actor/base"
)

type ChainCopyCmd struct {
	*base.Base
	Src  chain.ChainID `ask:"<source>" help:"The source, the chain to copy. Must exist."`
	Dest chain.ChainID `ask:"<dest>" help:"The destination, the name of the copy. Must not exist yet."`
}

func (c *ChainCopyCmd) Help() string {
	return "Copy an eth2 chain, used to maintain a different view of a chain than other actors on the original chain."
}

func (c *ChainCopyCmd) Run(ctx context.Context, args ...string) error {
	return errors.New("chain copying not implemented yet") // TODO
}
