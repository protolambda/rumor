package states

import (
	"context"
	"errors"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/sirupsen/logrus"
)

type SwitchCmd struct {
	*base.Base
	*DBState
	To sdb.DBID `ask:"<to>" help:"The name of the DB to switch to. Must exist."`
}

func (c *SwitchCmd) Help() string {
	return "Switch actor to another DB"
}

func (c *SwitchCmd) Run(ctx context.Context, args ...string) error {
	prev := c.DBState.CurrentDB
	if c.To == "" {
		return errors.New("need a DB name to switch to")
	}
	c.DBState.CurrentDB = c.To
	c.Log.WithFields(logrus.Fields{"from": prev, "to": c.To}).Info("switched DBs")
	return nil
}
