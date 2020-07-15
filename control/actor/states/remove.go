package states

import (
	"context"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/sirupsen/logrus"
)

type RemoveCmd struct {
	*base.Base
	sdb.DBs
	Name sdb.DBID `ask:"<name>" help:"The name of the DB to remove. Must exist."`
}

func (c *RemoveCmd) Help() string {
	return "Remove a DB"
}

func (c *RemoveCmd) Run(ctx context.Context, args ...string) error {
	existed := c.DBs.Remove(c.Name)
	c.Log.WithFields(logrus.Fields{"existed": existed, "name": c.Name}).Info("removed DB")
	return nil
}
