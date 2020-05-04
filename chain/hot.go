package chain

import (
	"context"
	"errors"
	"fmt"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/zrnt/eth2/forkchoice"
	"github.com/protolambda/ztyp/tree"
)

type HotEntry struct {
	slot       Slot
	epc        *beacon.EpochsContext
	state      *beacon.BeaconStateView
	blockRoot  Root
	parentRoot Root
}

func (e *HotEntry) Slot() Slot {
	return e.slot
}

func (e *HotEntry) ParentRoot() (root Root, ok bool) {
	return e.parentRoot, e.parentRoot == Root{}
}

func (e *HotEntry) BlockRoot() (root Root, here bool) {
	return e.blockRoot, e.parentRoot == Root{}
}

func (e *HotEntry) StateRoot() Root {
	return e.state.HashTreeRoot(tree.GetHashFn())
}

func (e *HotEntry) EpochsContext(ctx context.Context) (*beacon.EpochsContext, error) {
	return e.epc.Clone(), nil
}

func (e *HotEntry) State(ctx context.Context) (*beacon.BeaconStateView, error) {
	// Return a copy of the view, the state itself may not be modified
	return beacon.AsBeaconStateView(e.state.Copy())
}

type HotChain interface {
	Chain
	Justified() Checkpoint
	Finalized() Checkpoint
	Head() (ChainEntry, error)
	AddBlock(ctx context.Context, signedBlock *beacon.SignedBeaconBlock) error
	AddAttestation(att *beacon.Attestation) error
}

type UnfinalizedChain struct {
	ForkChoice *forkchoice.ForkChoice

	// block++slot -> Entry
	Entries map[BlockSlotKey]*HotEntry
	// state root -> block+slot key
	State2Key map[Root]BlockSlotKey
}

type BlockSink interface {
	// Sink handles blocks that come from the Hot part, and may be finalized or not
	Sink(block forkchoice.BlockRef, state *beacon.BeaconStateView, epc *beacon.EpochsContext, canonical bool)
}

type BlockSinkFn func(block forkchoice.BlockRef, state *beacon.BeaconStateView, epc *beacon.EpochsContext, canonical bool)

func (fn BlockSinkFn) Sink(block forkchoice.BlockRef, state *beacon.BeaconStateView, epc *beacon.EpochsContext, canonical bool) {
	fn(block, state, epc, canonical)
}

func NewUnfinalizedChain(finalizedBlock *HotEntry, sink BlockSink) (*UnfinalizedChain, error) {
	fin, err := finalizedBlock.state.FinalizedCheckpoint()
	if err != nil {
		return nil, err
	}
	finCh, err := fin.Raw()
	if err != nil {
		return nil, err
	}
	just, err := finalizedBlock.state.CurrentJustifiedCheckpoint()
	if err != nil {
		return nil, err
	}
	justCh, err := just.Raw()
	if err != nil {
		return nil, err
	}
	key := NewBlockSlotKey(finalizedBlock.blockRoot, finalizedBlock.slot)
	uc := &UnfinalizedChain{
		ForkChoice: nil,
		Entries: map[BlockSlotKey]*HotEntry{key: finalizedBlock},
		State2Key: map[Root]BlockSlotKey{finalizedBlock.StateRoot(): key},
	}
	fc, err := forkchoice.NewForkChoice(forkchoice.BlockRef{
		Slot: finalizedBlock.slot,
		Root: finalizedBlock.blockRoot,
	}, finCh, justCh, forkchoice.BlockSinkFn(func(node *forkchoice.ProtoNode, canonical bool) {
		entryId := NewBlockSlotKey(node.Block.Root, node.Block.Slot)
		// TODO concurrency safety
		entry, ok := uc.Entries[entryId]
		if ok {
			delete(uc.Entries, entryId)
			delete(uc.State2Key, entry.StateRoot())
			sink.Sink(node.Block, entry.state, entry.epc, canonical)
		}
		// TODO: log that we are pruning unrecognized blocks?
	}))
	if err != nil {
		return nil, err
	}
	uc.ForkChoice = fc
	return uc, nil
}

func (uc *UnfinalizedChain) OnPrunedBlock(node *forkchoice.ProtoNode) (pruned []ChainEntry) {
	blockRef := node.Block

	key := NewBlockSlotKey(blockRef.Root, blockRef.Slot)
	entry, ok := uc.Entries[key]
	if ok {
		// Return the block
		pruned = append(pruned, entry)
		// Remove block from hot state
		delete(uc.Entries, key)
		delete(uc.State2Key, entry.StateRoot())
		// There may be empty slots leading up to the block
		prevBlockRoot, _ := entry.ParentRoot()
		for slot := blockRef.Slot; true; slot-- {
			key = NewBlockSlotKey(prevBlockRoot, slot)
			entry, ok = uc.Entries[key]
			if ok {
				pruned = append(pruned, entry)
			} else {
				break
			}
		}
	}
	return
}

func (uc *UnfinalizedChain) ByStateRoot(root Root) (ChainEntry, error) {
	key, ok := uc.State2Key[root]
	if !ok {
		return nil, fmt.Errorf("unknown state %x", root)
	}
	return uc.ByBlockSlot(key)
}

func (uc *UnfinalizedChain) ByBlockSlot(key BlockSlotKey) (ChainEntry, error) {
	entry, ok := uc.Entries[key]
	if !ok {
		return nil, fmt.Errorf("unknown block slot, root: %x slot: %d", key.Root(), key.Slot())
	}
	return entry, nil
}

func (uc *UnfinalizedChain) ByBlockRoot(root Root) (ChainEntry, error) {
	ref, ok := uc.ForkChoice.GetBlock(root)
	if !ok {
		return nil, fmt.Errorf("unknown block %x", root)
	}
	return uc.ByBlockSlot(NewBlockSlotKey(root, ref.Slot))
}

func (uc *UnfinalizedChain) ClosestFrom(fromBlockRoot Root, toSlot Slot) (ChainEntry, error) {
	before, at, _, err := uc.ForkChoice.BlocksAroundSlot(fromBlockRoot, toSlot)
	if err != nil {
		return nil, err
	}
	if at.Root != (Root{}) {
		return uc.ByBlockSlot(NewBlockSlotKey(at.Root, at.Slot))
	}
	for slot := toSlot; slot >= before.Slot && slot != 0; slot-- {
		key := NewBlockSlotKey(before.Root, slot)
		entry, ok := uc.Entries[key]
		if ok {
			return entry, nil
		}
	}
	return nil, fmt.Errorf("could not find closest hot block starting from root %x, up to slot %d", fromBlockRoot, toSlot)
}

func (uc *UnfinalizedChain) BySlot(slot Slot) (ChainEntry, error) {
	_, at, _, err := uc.ForkChoice.BlocksAroundSlot(uc.Finalized().Root, slot)
	if err != nil {
		return nil, err
	}
	if at.Slot == slot {
		return uc.ByBlockSlot(NewBlockSlotKey(at.Root, at.Slot))
	}
	return nil, fmt.Errorf("no hot entry known for slot %d", slot)
}

func (uc *UnfinalizedChain) Justified() Checkpoint {
	return uc.ForkChoice.Justified()
}

func (uc *UnfinalizedChain) Finalized() Checkpoint {
	return uc.ForkChoice.Finalized()
}

func (uc *UnfinalizedChain) Head() (ChainEntry, error) {
	ref, err := uc.ForkChoice.FindHead()
	if err != nil {
		return nil, err
	}
	return uc.ByBlockRoot(ref.Root)
}

func (uc *UnfinalizedChain) AddBlock(ctx context.Context, signedBlock *beacon.SignedBeaconBlock) error {
	block := &signedBlock.Message
	blockRoot := block.HashTreeRoot()

	pre, err := uc.ClosestFrom(block.ParentRoot, block.Slot)
	if err != nil {
		return err
	}

	if root, ok := pre.BlockRoot(); !ok || root != block.ParentRoot {
		return fmt.Errorf("unknown parent root %x, found other root %x (ok: %v)", block.ParentRoot, root, ok)
	}

	epc, err := pre.EpochsContext(ctx)
	if err != nil {
		return err
	}

	state, err := pre.State(ctx)
	if err != nil {
		return err
	}

	// Process empty slots
	for slot := pre.Slot() + 1; slot < block.Slot; {
		if err := state.ProcessSlot(); err != nil {
			return err
		}
		// Per-epoch transition happens at the start of the first slot of every epoch.
		// (with the slot still at the end of the last epoch)
		isEpochEnd := (slot + 1).ToEpoch() != slot.ToEpoch()
		if isEpochEnd {
			if err := state.ProcessEpoch(epc); err != nil {
				return err
			}
		}
		slot += 1
		if err := state.SetSlot(slot); err != nil {
			return err
		}
		if isEpochEnd {
			if err := epc.RotateEpochs(state); err != nil {
				return err
			}
		}

		// Add empty slot entry
		uc.Entries[NewBlockSlotKey(block.ParentRoot, slot)] = &HotEntry{
			slot:       block.Slot,
			epc:        nil,
			state:      state,
			blockRoot:  blockRoot,
			parentRoot: Root{},
		}

		state, err = beacon.AsBeaconStateView(state.Copy())
		if err != nil {
			return err
		}
		epc = epc.Clone()
	}

	if err := state.StateTransition(epc, signedBlock, true); err != nil {
		return err
	}
	// And seal the state, need the header and block/state roots to update.
	if err := state.ProcessSlot(); err != nil {
		return err
	}

	var finalizedEpoch, justifiedEpoch Epoch
	{
		finalizedCh, err := state.FinalizedCheckpoint()
		if err != nil {
			return err
		}
		finalizedEpoch, err = finalizedCh.Epoch()
		if err != nil {
			return err
		}
		justifiedCh, err := state.CurrentJustifiedCheckpoint()
		if err != nil {
			return err
		}
		justifiedEpoch, err = justifiedCh.Epoch()
		if err != nil {
			return err
		}
	}

	uc.Entries[NewBlockSlotKey(blockRoot, block.Slot)] = &HotEntry{
		slot:       block.Slot,
		epc:        nil,
		state:      state,
		blockRoot:  blockRoot,
		parentRoot: block.ParentRoot,
	}
	uc.ForkChoice.ProcessBlock(
		forkchoice.BlockRef{Slot: block.Slot, Root: blockRoot},
		block.ParentRoot, justifiedEpoch, finalizedEpoch)
	return nil
}

func (uc *UnfinalizedChain) AddAttestation(att *beacon.Attestation) error {
	blockRoot := att.Data.BeaconBlockRoot
	block, err := uc.ByBlockRoot(blockRoot)
	if err != nil {
		return err
	}
	_, ok := block.(*HotEntry)
	if !ok {
		return errors.New("expected HotEntry, need epochs-context to be present")
	}
	// HotEntry does not use a context, epochs-context is available.
	epc, err := block.EpochsContext(nil)
	if err != nil {
		return err
	}
	committee, err := epc.GetBeaconCommittee(att.Data.Slot, att.Data.Index)
	if err != nil {
		return err
	}
	indexedAtt, err := att.ConvertToIndexed(committee)
	if err != nil {
		return err
	}
	targetEpoch := att.Data.Target.Epoch
	for _, index := range indexedAtt.AttestingIndices {
		uc.ForkChoice.ProcessAttestation(index, blockRoot, targetEpoch)
	}
	return nil
}
