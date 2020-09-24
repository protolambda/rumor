package chain

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/protolambda/zrnt/eth2/beacon"
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
	BlockRoot() (root Root)
	// The parent block root. If this is an empty slot, it will just be previous block root. Can also be zeroed if unknown.
	ParentRoot() (root Root)
	// If this is an empty slot, i.e. no block
	IsEmpty() bool
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
	Iter() (ChainIter, error)
}

type ChainIter interface {
	// Start is the minimum slot to reach to, inclusive.
	Start() Slot
	// End is the maximum slot to reach to, exclusive.
	End() Slot
	// Entry fetches the chain entry at the given slot.
	// If it does not exist, entry=nil, err=nil.
	// If the request is out of bounds or fails, an error may be returned.
	Entry(slot Slot) (entry ChainEntry, err error)
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

// TODO: in Go 1.15 this can be overlapping embedded interfaces
type FullChain interface {
	Chain

	// hot

	Justified() Checkpoint
	Finalized() Checkpoint
	Head() (ChainEntry, error)
	AddBlock(ctx context.Context, signedBlock *beacon.SignedBeaconBlock) error
	AddAttestation(att *beacon.Attestation) error

	// cold

	Start() Slot
	End() Slot
	OnFinalizedEntry(entry *HotEntry) error
}

type HotColdChain struct {
	HotChain
	ColdChain
	Spec *beacon.Spec
}

func (hc *HotColdChain) ByStateRoot(root Root) (ChainEntry, error) {
	hotEntry, hotErr := hc.HotChain.ByStateRoot(root)
	if hotErr != nil {
		coldEntry, coldErr := hc.ColdChain.ByStateRoot(root)
		if coldErr != nil {
			return nil, fmt.Errorf("could not find chain entry in hot or cold chain. "+
				"Hot: %v, Cold: %v", hotErr, coldErr)
		}
		return coldEntry, nil
	}
	return hotEntry, nil
}

func (hc *HotColdChain) ByBlockRoot(root Root) (ChainEntry, error) {
	hotEntry, hotErr := hc.HotChain.ByBlockRoot(root)
	if hotErr != nil {
		coldEntry, coldErr := hc.ColdChain.ByBlockRoot(root)
		if coldErr != nil {
			return nil, fmt.Errorf("could not find chain entry in hot or cold chain. "+
				"Hot: %v, Cold: %v", hotErr, coldErr)
		}
		return coldEntry, nil
	}
	return hotEntry, nil
}

func (hc *HotColdChain) ClosestFrom(fromBlockRoot Root, toSlot Slot) (ChainEntry, error) {
	hotEntry, hotErr := hc.HotChain.ClosestFrom(fromBlockRoot, toSlot)
	if hotErr != nil {
		coldEntry, coldErr := hc.ColdChain.ClosestFrom(fromBlockRoot, toSlot)
		if coldErr != nil {
			return nil, fmt.Errorf("could not find chain entry in hot or cold chain. "+
				"Hot: %v, Cold: %v", hotErr, coldErr)
		}
		return coldEntry, nil
	}
	return hotEntry, nil
}

func (hc *HotColdChain) BySlot(slot Slot) (ChainEntry, error) {
	hotEntry, hotErr := hc.HotChain.BySlot(slot)
	if hotErr != nil {
		coldEntry, coldErr := hc.ColdChain.BySlot(slot)
		if coldErr != nil {
			return nil, fmt.Errorf("could not find chain entry in hot or cold chain. "+
				"Hot: %v, Cold: %v", hotErr, coldErr)
		}
		return coldEntry, nil
	}
	return hotEntry, nil
}

func (hc *HotColdChain) Iter() (ChainIter, error) {
	hotIt, err := hc.HotChain.Iter()
	if err != nil {
		return nil, fmt.Errorf("cannot iter hot part: %v", err)
	}
	coldIt, err := hc.ColdChain.Iter()
	if err != nil {
		return nil, fmt.Errorf("cannot iter cold part: %v", err)
	}
	return &FullChainIter{
		HotIter:  hotIt,
		ColdIter: coldIt,
	}, nil
}

type FullChainIter struct {
	HotIter  ChainIter
	ColdIter ChainIter
}

func (fi *FullChainIter) Start() Slot {
	return fi.ColdIter.Start()
}

func (fi *FullChainIter) End() Slot {
	return fi.HotIter.End()
}

func (fi *FullChainIter) Entry(slot Slot) (entry ChainEntry, err error) {
	if slot < fi.ColdIter.End() {
		return fi.ColdIter.Entry(slot)
	} else {
		return fi.HotIter.Entry(slot)
	}
}

type ChainID string

type Chains interface {
	Find(id ChainID) (pi FullChain, ok bool)
	Create(id ChainID, anchor *HotEntry, spec *beacon.Spec) (pi FullChain, err error)
	Remove(id ChainID) (existed bool)
	List() []ChainID
}

type ChainsMap struct {
	// ChainID -> FullChain
	chains sync.Map
}

func (cs *ChainsMap) Find(id ChainID) (pi FullChain, ok bool) {
	pii, ok := cs.chains.Load(id)
	if !ok {
		return nil, false
	}
	return pii.(FullChain), true
}

func (cs *ChainsMap) Create(id ChainID, anchor *HotEntry, spec *beacon.Spec) (pi FullChain, err error) {
	coldCh := NewFinalizedChain(anchor.slot, spec)
	hotCh, err := NewUnfinalizedChain(anchor,
		BlockSinkFn(func(entry *HotEntry, canonical bool) error {
			if canonical {
				return coldCh.OnFinalizedEntry(entry)
			}
			return nil
			// TODO keep track of pruned non-finalized blocks?
		}), spec)
	if err != nil {
		return nil, err
	}

	c := &HotColdChain{
		HotChain:  hotCh,
		ColdChain: coldCh,
		Spec:      spec,
	}
	_, alreadyExisted := cs.chains.LoadOrStore(id, c)
	if alreadyExisted {
		return nil, errors.New("chain already existed")
	}
	return c, nil
}

func (cs *ChainsMap) Remove(id ChainID) (existed bool) {
	_, existed = cs.chains.Load(id)
	if existed {
		cs.chains.Delete(id)
	}
	return
}

func (cs *ChainsMap) List() (out []ChainID) {
	out = make([]ChainID, 0, 4)
	cs.chains.Range(func(key, value interface{}) bool {
		id := key.(ChainID)
		out = append(out, id)
		return true
	})
	return
}

// TODO: chain copy
