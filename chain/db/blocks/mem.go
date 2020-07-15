package blocks

import (
	"bytes"
	"context"
	"fmt"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/zrnt/eth2/util/ssz"
	"github.com/protolambda/zssz"
	"io"
	"sync"
	"sync/atomic"
)

type MemDB struct {
	// beacon.Root -> []byte (serialized SignedBeaconBlock)
	data        sync.Map
	removalLock sync.Mutex
	stats       DBStats
}

func (db *MemDB) Store(ctx context.Context, block *BlockWithRoot) (exists bool, err error) {
	// Released when the block is removed from the DB
	buf := getPoolBlockBuf()
	_, err = zssz.Encode(buf, block.Block, beacon.SignedBeaconBlockSSZ)
	if err != nil {
		return false, fmt.Errorf("failed to store block %s: %v", block.Root, err)
	}
	existing, loaded := db.data.LoadOrStore(block.Root, buf.Bytes())
	if loaded {
		existingBlock := existing.(*beacon.SignedBeaconBlock)
		sigDifference := existingBlock.Signature != block.Block.Signature
		dbBlockPool.Put(buf) // put it back, we didn't store it
		if sigDifference {
			return true, fmt.Errorf("block %s already exists, but its signature %x does not match new signature %s",
				block.Root, existingBlock.Signature, block.Block.Signature)
		}
	} else {
		atomic.AddInt64(&db.stats.Count, 1)
		db.stats.LastWrite = block.Root
	}
	return loaded, nil
}

func (db *MemDB) Import(r io.Reader) (exists bool, err error) {
	buf := getPoolBlockBuf()
	if _, err := buf.ReadFrom(r); err != nil {
		dbBlockPool.Put(buf) // put it back, we didn't use it
		return false, err
	}
	var dest beacon.SignedBeaconBlock
	err = zssz.Decode(buf, uint64(len(buf.Bytes())), &dest, beacon.SignedBeaconBlockSSZ)
	if err != nil {
		return false, fmt.Errorf("failed to decode block, nee valid block to get block root. Err: %v", err)
	}
	// Take the hash-tree-root of the BeaconBlock, ignore the signature.
	root := beacon.Root(ssz.HashTreeRoot(&dest.Message, beacon.BeaconBlockSSZ))
	existing, loaded := db.data.LoadOrStore(root, buf.Bytes())
	if loaded {
		existingBlock := existing.(*beacon.SignedBeaconBlock)
		sigDifference := existingBlock.Signature != dest.Signature
		dbBlockPool.Put(buf) // put it back, we didn't store it
		if sigDifference {
			return true, fmt.Errorf("block %s already exists, but its signature %s does not match new signature %s",
				root, existingBlock.Signature, dest.Signature)
		}
	} else {
		atomic.AddInt64(&db.stats.Count, 1)
		db.stats.LastWrite = root
	}
	return loaded, nil
}

func (db *MemDB) Get(root beacon.Root, dest *beacon.SignedBeaconBlock) (exists bool, err error) {
	dat, ok := db.data.Load(root)
	if !ok {
		return false, nil
	}
	buf := dat.(*bytes.Buffer)
	err = zssz.Decode(buf, uint64(len(buf.Bytes())), dest, beacon.SignedBeaconBlockSSZ)
	return true, err
}

func (db *MemDB) Size(root beacon.Root) (size uint64, exists bool) {
	dat, ok := db.data.Load(root)
	if !ok {
		return 0, false
	}
	buf := dat.(*bytes.Buffer)
	return uint64(len(buf.Bytes())), true
}

func (db *MemDB) Export(root beacon.Root, w io.Writer) (exists bool, err error) {
	dat, ok := db.data.Load(root)
	if !ok {
		return false, nil
	}
	buf := dat.(*bytes.Buffer)
	_, err = buf.WriteTo(w)
	return true, err
}

type noClose struct {
	io.Reader
}

func (n noClose) Close() error {
	return nil
}

func (db *MemDB) Stream(root beacon.Root) (r io.ReadCloser, size uint64, exists bool, err error) {
	dat, ok := db.data.Load(root)
	if !ok {
		return nil, 0, false, nil
	}
	buf := dat.(*bytes.Buffer)
	return noClose{buf}, uint64(buf.Len()), true, nil
}

func (db *MemDB) Remove(root beacon.Root) (exists bool, err error) {
	db.removalLock.Lock()
	defer db.removalLock.Unlock()
	v, ok := db.data.Load(root)
	if ok {
		dbBlockPool.Put(v) // release it back to pool, it's not used in the DB anymore.
		atomic.AddInt64(&db.stats.Count, -1)
	}
	db.data.Delete(root)
	return ok, nil
}

func (db *MemDB) Stats() DBStats {
	// return a copy (struct is small and has no pointers)
	return db.stats
}

func (db *MemDB) List() (out []beacon.Root) {
	out = make([]beacon.Root, 0, db.stats.Count)
	db.data.Range(func(key, value interface{}) bool {
		id := key.(beacon.Root)
		out = append(out, id)
		return true
	})
	return out
}

func (db *MemDB) Path() string {
	return ""
}
