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

type BlockWithRoot struct {
	// Root of the Block.Message
	Root  beacon.Root
	// Block, with signature
	Block *beacon.SignedBeaconBlock
}

func WithRoot(block *beacon.SignedBeaconBlock) *BlockWithRoot {
	root := beacon.Root(ssz.HashTreeRoot(&block.Message, beacon.BeaconBlockSSZ))
	return &BlockWithRoot{
		Root:  root,
		Block: block,
	}
}

type DBStats struct {
	Count     int64
	LastWrite beacon.Root
}

type DB interface {
	// Store, only for trusted blocks, to persist a block in the DB.
	// The block is stored in serialized form, so the original instance may be mutated after storing it.
	// This is an efficient convenience method for using Import.
	// Returns exists=true if the block exists (previously), false otherwise. If error, it may not be accurate.
	// Returns slashable=true if exists=true, but the signatures are different. The existing block is kept.
	Store(ctx context.Context, block *BlockWithRoot) (exists bool, err error)
	// Import inserts a SignedBeaconBlock, read directly from the reader stream.
	// Returns exists=true if the block exists (previously), false otherwise. If error, it may not be accurate.
	// Returns slashable=true if exists=true, but the signatures are different. The existing block is kept.
	Import(r io.Reader) (exists bool, err error)
	// Get, an efficient convenience method for getting a block through Export. The block is safe to modify.
	// The data at the pointer is mutated to the new block.
	// Returns exists=true if the block exists, false otherwise. If error, it may not be accurate.
	Get(root beacon.Root, dest *beacon.SignedBeaconBlock) (exists bool, err error)
	// Size quickly checks the size of a block, without dealing with the full block.
	// Returns exists=true if the block exists, false otherwise. If error, it may not be accurate.
	Size(root beacon.Root) (size uint64, exists bool)
	// Export outputs the requested SignedBeaconBlock to the writer in SSZ.
	// Returns exists=true if the block exists, false otherwise. If error, it may not be accurate.
	Export(root beacon.Root, w io.Writer) (exists bool, err error)
	// Remove removes a block from the DB. Removing a block that does not exist is safe.
	// Returns exists=true if the block exists (previously), false otherwise. If error, it may not be accurate.
	Remove(root beacon.Root) (exists bool, err error)
	// Stats shows some database statistics such as latest write key and entry count.
	Stats() DBStats
}

type MemDB struct {
	// beacon.Root -> []byte (serialized SignedBeaconBlock)
	data        sync.Map
	removalLock sync.Mutex
	stats       DBStats
}

var maxBlockSize = beacon.SignedBeaconBlockSSZ.MaxLen()

var dbBlockPool = sync.Pool{
	New: func() interface{} {
		// ensure enough capacity for any block. We pool it anyway, so eventually it may grow that big.
		return bytes.NewBuffer(make([]byte, 0, maxBlockSize))
	},
}

func getPoolBlockBuf() *bytes.Buffer {
	buf := dbBlockPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

func (db *MemDB) Store(ctx context.Context, block *BlockWithRoot) (exists bool, err error) {
	// Released when the block is removed from the DB
	buf := getPoolBlockBuf()
	_, err = zssz.Encode(buf, block.Block, beacon.SignedBeaconBlockSSZ)
	if err != nil {
		return false, fmt.Errorf("failed to store block %x: %v", block.Root, err)
	}
	existing, loaded := db.data.LoadOrStore(block.Root, buf.Bytes())
	if loaded {
		existingBlock := existing.(*beacon.SignedBeaconBlock)
		sigDifference := existingBlock.Signature != block.Block.Signature
		dbBlockPool.Put(buf) // put it back, we didn't store it
		if sigDifference {
			return true, fmt.Errorf("block %x already exists, but its signature %x does not match new signature %x",
				block.Root[:], existingBlock.Signature[:], block.Block.Signature[:])
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
			return true, fmt.Errorf("block %x already exists, but its signature %x does not match new signature %x",
				root[:], existingBlock.Signature[:], dest.Signature[:])
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
