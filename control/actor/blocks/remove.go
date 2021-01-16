package blocks

import (
	"context"
	"github.com/protolambda/rumor/dbs"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/sirupsen/logrus"
)

type RemoveCmd struct {
	*base.Base
	dbs.BlocksDBs
	Name dbs.BlocksDBID `ask:"<name>" help:"The name of the DB to remove. Must exist."`
}

func (c *RemoveCmd) Help() string {
	return "Remove a DB"
}

func (c *RemoveCmd) Run(ctx context.Context, args ...string) error {
	existed := c.BlocksDBs.Remove(c.Name)
	c.Log.WithFields(logrus.Fields{"existed": existed, "name": c.Name}).Info("removed DB")
	return nil
}
