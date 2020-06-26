package chain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/protolambda/rumor/chain"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/ztyp/tree"
	"io"
	"os"
)

type ChainCreateCmd struct {
	*base.Base
	*chain.Chains
	Name       chain.ChainID `ask:"<name>" help:"The name to give to the created chain. Must not exist yet."`
	StateInput string        `ask:"--state-file" help:"State input file"`
	StateData  []byte        `ask:"--state-data" help:"Alternative to state input, read state from bytes directly"`
}

func (c *ChainCreateCmd) Help() string {
	return "Create a new eth2 chain from a pre-state"
}

func (c *ChainCreateCmd) Run(ctx context.Context, args ...string) error {
	var state *beacon.BeaconStateView
	{
		var r io.Reader
		var size uint64
		if c.StateInput == "" {
			if len(c.StateData) == 0 {
				return errors.New("no anchor state specified for chain")
			}
			r = bytes.NewReader(c.StateData)
			size = uint64(len(c.StateData))
		} else {
			f, err := os.OpenFile(c.StateInput, os.O_RDONLY, os.ModePerm)
			if err != nil {
				return fmt.Errorf("failed to open %s: %v", c.StateInput, err)
			}
			defer f.Close()
			r = f
		}
		var err error
		state, err = beacon.AsBeaconStateView(beacon.BeaconStateType.Deserialize(r, size))
		if err != nil {
			return err
		}
	}
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
	stateRoot, err := latestHeader.StateRoot()
	if err != nil {
		return err
	}
	if stateRoot == (beacon.Root{}) {
		if err := latestHeader.SetStateRoot(state.HashTreeRoot(tree.GetHashFn())); err != nil {
			return err
		}
	}
	blockRoot := latestHeader.HashTreeRoot(tree.GetHashFn())
	parentRoot, err := latestHeader.ParentRoot()
	if err != nil {
		return err
	}
	epc, err := state.NewEpochsContext()
	if err != nil {
		return err
	}
	entry := chain.NewHotEntry(slot, blockRoot, parentRoot, state, epc)
	_, err = c.Chains.Create(c.Name, entry)
	return err
}
