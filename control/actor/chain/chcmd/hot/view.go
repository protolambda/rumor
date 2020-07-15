package hot

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
)

type ViewCmd struct {
	*base.Base
	Anchor beacon.Root `ask:"--anchor" help:"anchor root of subtree to view"`
}

func (c *ViewCmd) Help() string {
	return "View (a subtree of) the hot chain."
}

func (c *ViewCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
