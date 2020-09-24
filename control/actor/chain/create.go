package chain

import (
	"context"
	"fmt"
	"github.com/protolambda/rumor/chain"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/ztyp/tree"
)

type ChainCreateCmd struct {
	*base.Base
	chain.Chains
	*ChainState
	States    sdb.DB
	Name      chain.ChainID `ask:"<name>" help:"The name to give to the created chain. Must not exist yet."`
	StateRoot beacon.Root   `ask:"<state>" help:"The state to start from, retrieved from the states DB"`
}

func (c *ChainCreateCmd) Help() string {
	return "Create a new eth2 chain from a pre-state (using same spec config as state)"
}

func (c *ChainCreateCmd) Run(ctx context.Context, args ...string) error {
	state, exists, err := c.States.Get(c.StateRoot)
	if err != nil {
		return fmt.Errorf("failed to get state: %v", err)
	}
	if !exists {
		return fmt.Errorf("state %s was not found", c.StateRoot)
	}
	spec := c.States.Spec()
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
	epc, err := spec.NewEpochsContext(state)
	if err != nil {
		return err
	}
	entry := chain.NewHotEntry(slot, blockRoot, parentRoot, state, epc)
	_, err = c.Chains.Create(c.Name, entry, spec)
	if err != nil {
		return err
	}
	c.ChainState.CurrentChain = c.Name
	return nil
}
