package dbcmd

import (
	"context"
	"encoding/hex"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/sirupsen/logrus"
)

type BlocksStatsCmd struct {
	*base.Base
	bdb.DB
}

func (c *BlocksStatsCmd) Help() string {
	return "Show stats of currently managed blocks"
}

func (c *BlocksStatsCmd) Run(ctx context.Context, args ...string) error {
	stats := c.DB.Stats()
	c.Log.WithFields(logrus.Fields{"count": stats.Count, "last": hex.EncodeToString(stats.LastWrite[:])}).Info("blocks DB stats")
	return nil
}
