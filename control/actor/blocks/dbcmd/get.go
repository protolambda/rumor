package dbcmd

import (
	"context"
	"encoding/hex"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
)

type BlocksGetCmd struct {
	*base.Base
	bdb.DB
	BlockRoot beacon.Root `ask:"<root>" help:"Root of the block to get"`
}

func (c *BlocksGetCmd) Help() string {
	return "Get a summary of a SignedBeaconBlock by its block root"
}

func (c *BlocksGetCmd) Run(ctx context.Context, args ...string) error {
	var block beacon.SignedBeaconBlock
	exists, err := c.DB.Get(c.BlockRoot, &block)
	if err != nil {
		return err
	}
	if exists {
		c.Log.WithField("block", map[string]interface{}{
			"slot":      block.Message.Slot,
			"proposer":  block.Message.ProposerIndex,
			"parent":    hex.EncodeToString(block.Message.ParentRoot[:]),
			"state":     hex.EncodeToString(block.Message.StateRoot[:]),
			"signature": hex.EncodeToString(block.Signature[:]),
		}).Info("got block")
	} else {
		c.Log.Warnf("block %s is not known", c.BlockRoot)
	}
	return nil
}
