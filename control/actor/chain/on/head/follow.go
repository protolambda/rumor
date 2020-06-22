package head

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
)

type FollowCmd struct {
	*base.Base
	Following bool `ask:"<following>" help:"If the head of the chain should automatically be followed"`
}

func (c *FollowCmd) Default() {
	c.Following = true
}

func (c *FollowCmd) Help() string {
	return "Follow the head of the chain or not."
}

func (c *FollowCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
