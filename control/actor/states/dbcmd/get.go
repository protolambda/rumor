package dbcmd

import (
	"context"
	"encoding/hex"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/ztyp/tree"
)

type StatesGetCmd struct {
	*base.Base
	sdb.DB
	StateRoot beacon.Root `ask:"<root>" help:"Root of the state to get"`
}

func (c *StatesGetCmd) Help() string {
	return "Get a summary of a SignedBeaconBlock by its block root"
}

func (c *StatesGetCmd) Run(ctx context.Context, args ...string) error {
	state, exists, err := c.DB.Get(c.StateRoot)
	if err != nil {
		return err
	}
	if exists {
		slot, err := state.Slot()
		if err != nil {
			return err
		}
		latestHeader, err := state.LatestBlockHeader()
		if err != nil {
			return err
		}
		latestHeader, err = beacon.AsBeaconBlockHeader(latestHeader.Copy())
		if err != nil {
			return err
		}
		headerStateRoot, err := latestHeader.StateRoot()
		if err != nil {
			return err
		}
		if headerStateRoot == (beacon.Root{}) {
			if err := latestHeader.SetStateRoot(state.HashTreeRoot(tree.GetHashFn())); err != nil {
				return err
			}
		}
		blockRoot := latestHeader.HashTreeRoot(tree.GetHashFn())
		parentRoot, err := latestHeader.ParentRoot()
		if err != nil {
			return err
		}
		c.Log.WithField("state", map[string]interface{}{
			"slot":         slot,
			"latest_block": hex.EncodeToString(blockRoot[:]),
			"parent_block": hex.EncodeToString(parentRoot[:]),
		}).Info("got state")
	} else {
		c.Log.Warnf("state %s is not known", c.StateRoot)
	}
	return nil
}
