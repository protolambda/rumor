package blocks

import (
	"context"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/ztyp/tree"
	"sync"
	"sync/atomic"
)

type DBStats struct {
	Count     int64
	LastWrite beacon.Root
}

type DB interface {
	// Store a state.
	// Returns exists=true if the state exists (previously), false otherwise. If error, it may not be accurate.
	Store(ctx context.Context, state *beacon.BeaconStateView) (exists bool, err error)
	// Get a state. The state is a view of a shared immutable backing.
	// The view is save to mutate (it forks away from the original backing)
	// If the state does not exist, a nil state is returned without error.
	Get(root beacon.Root) (state *beacon.BeaconStateView, err error)
	// Remove removes a state from the DB. Removing a state that does not exist is safe.
	// Returns exists=true if the state exists (previously), false otherwise. If error, it may not be accurate.
	Remove(root beacon.Root) (exists bool, err error)
	// Stats shows some database statistics such as latest write key and entry count.
	Stats() DBStats
}

type MemDB struct {
	// beacon.Root -> tree.Node (backing of BeaconStateView)
	data        sync.Map
	removalLock sync.Mutex
	stats       DBStats
}

func (db *MemDB) Store(ctx context.Context, state *beacon.BeaconStateView) (exists bool, err error) {
	// Released when the block is removed from the DB
	root := state.HashTreeRoot(tree.GetHashFn())
	backing := state.Backing()
	_, loaded := db.data.LoadOrStore(root, backing)
	if !loaded {
		atomic.AddInt64(&db.stats.Count, 1)
		db.stats.LastWrite = root
	}
	return loaded, nil
}

func (db *MemDB) Get(root beacon.Root) (state *beacon.BeaconStateView, err error) {
	dat, ok := db.data.Load(root)
	if !ok {
		return nil, nil
	}
	v, vErr := beacon.BeaconStateType.ViewFromBacking(dat.(tree.Node), nil)
	state, err = beacon.AsBeaconStateView(v, vErr)
	return
}

func (db *MemDB) Remove(root beacon.Root) (exists bool, err error) {
	db.removalLock.Lock()
	defer db.removalLock.Unlock()
	_, ok := db.data.Load(root)
	if ok {
		atomic.AddInt64(&db.stats.Count, -1)
	}
	db.data.Delete(root)
	return ok, nil
}

func (db *MemDB) Stats() DBStats {
	// return a copy (struct is small and has no pointers)
	return db.stats
}
