package dbcmd

import (
	"context"
	"encoding/hex"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/sirupsen/logrus"
)

type StatesStatsCmd struct {
	*base.Base
	sdb.DB
}

func (c *StatesStatsCmd) Help() string {
	return "Show stats of currently managed states"
}

func (c *StatesStatsCmd) Run(ctx context.Context, args ...string) error {
	stats := c.DB.Stats()
	c.Log.WithFields(logrus.Fields{"count": stats.Count, "last": hex.EncodeToString(stats.LastWrite[:])}).Info("states DB stats")
	return nil
}
