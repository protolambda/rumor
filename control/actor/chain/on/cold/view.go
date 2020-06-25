package cold

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
)

type ViewCmd struct {
	*base.Base
	Start beacon.Slot `ask:"--start" help:"Starting point (inclusive)"`
	End   beacon.Slot `ask:"--end" help:"End point (exclusive)"`
}

func (c *ViewCmd) Help() string {
	return "View (a range of) the cold chain."
}

func (c *ViewCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
