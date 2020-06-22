package blocks

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
)

type BlocksStatsCmd struct {
	*base.Base
}

func (c *BlocksStatsCmd) Help() string {
	return "Show stats of currently managed blocks"
}

func (c *BlocksStatsCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
