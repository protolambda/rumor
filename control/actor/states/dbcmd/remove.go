package dbcmd

import (
	"context"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
)

type StatesRemoveCmd struct {
	*base.Base
	sdb.DB
	StateRoot beacon.Root `ask:"<root>" help:"Root of the state to delete"`
}

func (c *StatesRemoveCmd) Help() string {
	return "Remove a block from the managed States collection"
}

func (c *StatesRemoveCmd) Run(ctx context.Context, args ...string) error {
	exists, err := c.DB.Remove(c.StateRoot)
	if err != nil {
		return err
	}
	c.Log.WithField("existed", exists).Infof("removed state")
	return nil
}
