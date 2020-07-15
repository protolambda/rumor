package states

import (
	"context"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
)

type ListCmd struct {
	*base.Base
	sdb.DBs
	*DBState
}

func (c *ListCmd) Help() string {
	return "List DBs and identify current DB"
}

func (c *ListCmd) Run(ctx context.Context, args ...string) error {
	dbIDs := c.DBs.List()
	c.Log.WithField("dbs", dbIDs).Infof("Got %d DBs", len(dbIDs))

	if current := c.DBState.CurrentDB; current != "" {
		db, ok := c.DBs.Find(current)
		if ok {
			c.Log.WithField("current", current).WithField("path", db.Path()).Info("Current DB")
		} else {
			c.Log.WithField("current", "").Info("Current DB cannot be found")
		}
	} else {
		c.Log.WithField("current", "").Info("No current DB")
	}
	return nil
}
