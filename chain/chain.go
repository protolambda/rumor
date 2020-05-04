package chain

import (
	"context"
	"encoding/binary"
	"errors"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/zrnt/eth2/forkchoice"
	"sync"
)

type Root = beacon.Root
type Epoch = beacon.Epoch
type Slot = beacon.Slot
type ValidatorIndex = beacon.ValidatorIndex
type Gwei = beacon.Gwei
type Checkpoint = beacon.Checkpoint

type ChainEntry interface {
	// Slot of this entry
	Slot() Slot
	// BlockRoot returns the last block root, replicating the previous block root if the current slot has none.
	// If replicated, `here` will be false.
	BlockRoot() (root Root, here bool)
	// The parent block root. If this is an empty slot, return ok=false.
	ParentRoot() (root Root, ok bool)
	// State root (of the post-state of this entry). Should match state-root in the block at the same slot (if any)
	StateRoot() Root
	// The context of this chain entry (shuffling, proposers, etc.)
	EpochsContext(ctx context.Context) (*beacon.EpochsContext, error)
	// State of the entry, a data-sharing view. Call .Copy() before modifying this to preserve validity.
	State(ctx context.Context) (*beacon.BeaconStateView, error)
}

type Chain interface {
	ByStateRoot(root Root) (ChainEntry, error)
	ByBlockRoot(root Root) (ChainEntry, error)
	ClosestFrom(fromBlockRoot Root, toSlot Slot) (ChainEntry, error)
	BySlot(slot Slot) (ChainEntry, error)
}

type BlockSlotKey [32 + 8]byte

func (key *BlockSlotKey) Slot() Slot {
	return Slot(binary.LittleEndian.Uint64(key[32:40]))
}

func (key *BlockSlotKey) Root() (out Root) {
	copy(out[:], key[0:32])
	return
}

func NewBlockSlotKey(block Root, slot Slot) (out BlockSlotKey) {
	copy(out[0:32], block[:])
	binary.LittleEndian.PutUint64(out[32:40], uint64(slot))
	return
}

type FullChain struct {
	HotChain
	ColdChain
}

// TODO: call hot/cold functions based on input + finalized slot

type ChainID string

type Chains struct {
	// ChainID -> *FullChain
	chains sync.Map
}

func (cs *Chains) Find(id ChainID) (pi *FullChain, ok bool) {
	pii, ok := cs.chains.Load(id)
	return pii.(*FullChain), ok
}

func (cs *Chains) Create(id ChainID, anchor *HotEntry) (pi *FullChain, err error) {
	// TODO: genesis?
	coldCh := NewFinalizedChain(anchor.slot)
	hotCh, err := NewUnfinalizedChain(anchor,
		BlockSinkFn(func(
			block forkchoice.BlockRef, state *beacon.BeaconStateView,
			epc *beacon.EpochsContext, canonical bool) {
		if canonical {
			coldCh.OnFinalizedBlock(block, state, epc)
		}
		// TODO keep track of pruned non-finalized blocks?
	}))
	if err != nil {
		return nil, err
	}

	c := &FullChain{
		HotChain: hotCh,
		ColdChain: coldCh,
	}
	_, alreadyExisted := cs.chains.LoadOrStore(id, c)
	if alreadyExisted {
		return nil, errors.New("chain already existed")
	}
	return c, nil
}
