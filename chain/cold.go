package chain

import (
	"context"
	"errors"
	"fmt"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/zrnt/eth2/forkchoice"
	"github.com/protolambda/ztyp/tree"
)

type ColdEntry struct {
	slot Slot

	// Can be copy of the previous root if empty
	blockRoot Root
	// Zeroed if empty.
	parentRoot Root

	stateRoot Root
}

func (e *ColdEntry) Slot() Slot {
	return e.slot
}

func (e *ColdEntry) ParentRoot() (root Root, ok bool) {
	return e.parentRoot, e.parentRoot == Root{}
}

func (e *ColdEntry) BlockRoot() (root Root, here bool) {
	return e.blockRoot, e.parentRoot == (Root{})
}

func (e *ColdEntry) StateRoot() Root {
	return e.stateRoot
}

type ColdChain interface {
	Start() Slot
	End() Slot
	OnFinalizedBlock(block forkchoice.BlockRef, state *beacon.BeaconStateView, epc *beacon.EpochsContext)
	Chain
}

type FinalizedEntryView struct {
	*ColdEntry
	finChain *FinalizedChain
}

func (e *FinalizedEntryView) EpochsContext(ctx context.Context) (*beacon.EpochsContext, error) {
	return e.finChain.getEpochsContext(ctx, e.slot)
}

func (e *FinalizedEntryView) State(ctx context.Context) (*beacon.BeaconStateView, error) {
	return e.finChain.getState(ctx, e.slot)
}

type FinalizedChain struct {
	// Cache of pubkeys, may contain pubkeys that are not finalized,
	// but finalized state will not be in conflict with this cache.
	PubkeyCache *beacon.PubkeyCache
	// Start of the historical data
	AnchorSlot Slot
	// History stores canonical chain by slot, starting at AnchorSlot
	History []ColdEntry
	// Proposers, grouped by epoch, starting from AnchorSlot.ToEpoch()
	ProposerHistory [][beacon.SLOTS_PER_EPOCH]ValidatorIndex
	// BlockRoots maps the canonical chain block roots to the block slot
	BlockRoots map[Root]Slot
	// BlockRoots maps the canonical chain state roots to the state slot
	StateRoots map[Root]Slot
}

var _ = ColdChain((*FinalizedChain)(nil))

func NewFinalizedChain(anchorSlot Slot) *FinalizedChain {
	initialCapacity := Slot(200)
	return &FinalizedChain{
		PubkeyCache:     beacon.EmptyPubkeyCache(),
		AnchorSlot:      anchorSlot,
		History:         make([]ColdEntry, 0, initialCapacity),
		ProposerHistory: make([][beacon.SLOTS_PER_EPOCH]ValidatorIndex, 0, initialCapacity.ToEpoch()),
		BlockRoots:      make(map[Root]Slot, initialCapacity),
		StateRoots:      make(map[Root]Slot, initialCapacity),
	}
}

func (f *FinalizedChain) Start() Slot {
	return f.AnchorSlot
}

func (f *FinalizedChain) End() Slot {
	return f.AnchorSlot + Slot(len(f.History))
}

var UnknownRootErr = errors.New("unknown root")

func (f *FinalizedChain) ByStateRoot(root Root) (ChainEntry, error) {
	slot, ok := f.StateRoots[root]
	if !ok {
		return nil, UnknownRootErr
	}
	return f.BySlot(slot)
}

func (f *FinalizedChain) ByBlockRoot(root Root) (ChainEntry, error) {
	slot, ok := f.StateRoots[root]
	if !ok {
		return nil, UnknownRootErr
	}
	return f.BySlot(slot)
}

func (f *FinalizedChain) ClosestFrom(fromBlockRoot Root, toSlot Slot) (ChainEntry, error) {
	if start := f.Start(); toSlot < start {
		return nil, fmt.Errorf("slot %d is too early. Start is at slot %d", toSlot, start)
	}
	// check if the root is canonical
	_, ok := f.StateRoots[fromBlockRoot]
	if !ok {
		return nil, UnknownRootErr
	}
	// find the slot closest to the requested slot: whatever is still within range
	if end := f.End(); end == 0 {
		return nil, errors.New("empty chain, no data available")
	} else if toSlot >= end {
		toSlot = end - 1
	}
	return f.BySlot(toSlot)
}

func (f *FinalizedChain) BySlot(slot Slot) (ChainEntry, error) {
	if start := f.AnchorSlot; slot < start {
		return nil, fmt.Errorf("slot %d is too early. Chain starts at slot %d", slot, start)
	}
	if end := f.AnchorSlot + Slot(len(f.History)); slot >= end {
		return nil, fmt.Errorf("slot %d is too late. Chain ends at slot %d", slot, end)
	}
	return &FinalizedEntryView{
		ColdEntry: &f.History[slot-f.AnchorSlot],
		finChain:  f,
	}, nil
}

func (f *FinalizedChain) OnFinalizedBlock(block forkchoice.BlockRef, state *beacon.BeaconStateView, epc *beacon.EpochsContext) {
	postStateRoot := state.HashTreeRoot(tree.GetHashFn())
	f.History = append(f.History, ColdEntry{
		slot:      block.Slot,
		blockRoot: block.Root,
		stateRoot: postStateRoot,
	})
	// If it's a new epoch, store the proposers for the epoch
	if f.AnchorSlot.ToEpoch() + Epoch(len(f.ProposerHistory)) <= block.Slot.ToEpoch() {
		f.ProposerHistory = append(f.ProposerHistory, *epc.Proposers)
	}
	f.BlockRoots[block.Root] = block.Slot
	f.StateRoots[postStateRoot] = block.Slot
}

func (f *FinalizedChain) Proposers(epoch Epoch) (*[beacon.SLOTS_PER_EPOCH]ValidatorIndex, error) {
	anchorEpoch := f.AnchorSlot.ToEpoch()
	if epoch < anchorEpoch {
		return nil, errors.New("epoch too early for finalized chain")
	}
	if max := f.End().ToEpoch(); epoch >= max {
		return nil, errors.New("epoch too late for finalized chain")
	}
	return &f.ProposerHistory[epoch-anchorEpoch], nil
}

func (f *FinalizedChain) getEpochsContext(ctx context.Context, slot Slot) (*beacon.EpochsContext, error) {
	// TODO: context can be partially constructed from contexts around this.
	proposers, err := f.Proposers(slot.ToEpoch())
	if err != nil {
		return nil, err
	}
	epc := &beacon.EpochsContext{
		PubkeyCache: f.PubkeyCache,
		Proposers:   proposers,
	}
	// We do not store shuffling for older epochs
	// TODO: maybe store it after all, for archive node functionality?
	state, err := f.getState(ctx, slot)
	if err != nil {
		return nil, err
	}
	if err := epc.LoadShuffling(state); err != nil {
		return nil, err
	}
	return epc, nil
}

func (f *FinalizedChain) getState(ctx context.Context, slot Slot) (*beacon.BeaconStateView, error) {
	return nil, errors.New("todo: load state from state storage")
}
