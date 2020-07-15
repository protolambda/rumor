package dbcmd

import (
	"context"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
)

type StatesListCmd struct {
	*base.Base
	sdb.DB
}

func (c *StatesListCmd) Help() string {
	return "List known state roots"
}

func (c *StatesListCmd) Run(ctx context.Context, args ...string) error {
	roots := c.DB.List()
	c.Log.WithField("state_roots", roots).Infof("got %d state roots", len(roots))
	return nil
}
