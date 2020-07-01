package dstrack

import (
	"bytes"
	"encoding/binary"
	"fmt"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/protolambda/zssz"
	"sync"
)

var (
	metadataSuffix  = ds.NewKey("/metadata")
	claimSuffix  = ds.NewKey("/metadata_claim")
)

type dsMetadataBook struct {
	ds ds.Datastore
	// cache metadata objects to not load/store them all the time
	sync.RWMutex
	// Track metadata with highest sequence number
	metadatas map[peer.ID]methods.MetaData
	// highest claimed seq nr, we may not have the actual corresponding metadata yet.
	claims map[peer.ID]methods.SeqNr
	// Track how many times we have tried to ask them for metadata without getting an answer
	fetches map[peer.ID]uint64
}

var _ track.MetadataBook = (*dsMetadataBook)(nil)

func NewMetadataBook(store ds.Datastore) (*dsMetadataBook, error) {
	return &dsMetadataBook{ds: store}, nil
}

func (mb *dsMetadataBook) loadMetadata(p peer.ID) (*methods.MetaData, error) {
	key := peerIdToKey(p).Child(metadataSuffix)
	value, err := mb.ds.Get(key)
	if err != nil {
		return nil, fmt.Errorf("error while fetching metadata from datastore for peer %s: %s\n", p.Pretty(), err)
	}
	var md methods.MetaData
	if err := zssz.Decode(bytes.NewReader(value), uint64(len(value)), &md, methods.MetaDataSSZ); err != nil {
		return nil, fmt.Errorf("failed parse metadata bytes from datastore: %v", err)
	}
	return &md, nil
}

func (mb *dsMetadataBook) storeMetadata(p peer.ID, md *methods.MetaData) error {
	key := peerIdToKey(p).Child(metadataSuffix)
	size := zssz.SizeOf(md, methods.MetaDataSSZ)
	out := bytes.NewBuffer(make([]byte, size, size))
	if _, err := zssz.Encode(out, md, methods.MetaDataSSZ); err != nil {
		return fmt.Errorf("failed encode metadata bytes for datastore: %v", err)
	}
	if err := mb.ds.Put(key, out.Bytes()); err != nil {
		return fmt.Errorf("failed to store metadata: %v", err)
	}
	return nil
}

func (mb *dsMetadataBook) loadClaim(p peer.ID) (methods.SeqNr, error) {
	key := peerIdToKey(p).Child(claimSuffix)
	value, err := mb.ds.Get(key)
	if err != nil {
		return 0, fmt.Errorf("error while fetching claim seq nr from datastore for peer %s: %s\n", p.Pretty(), err)
	}
	claim := methods.SeqNr(binary.LittleEndian.Uint64(value))
	return claim, nil
}

func (mb *dsMetadataBook) storeClaim(p peer.ID, claim methods.SeqNr) error {
	key := peerIdToKey(p).Child(claimSuffix)
	var dat [8]byte
	binary.LittleEndian.PutUint64(dat[:], uint64(claim))
	if err := mb.ds.Put(key, dat[:]); err != nil {
		return fmt.Errorf("failed to store claim seq nr: %v", err)
	}
	return nil
}

func (mb *dsMetadataBook) Metadata(id peer.ID) *methods.MetaData {
	mb.RLock()
	dat, ok := mb.metadatas[id]
	mb.RUnlock()
	if !ok {
		md, err := mb.loadMetadata(id)
		if err != nil {
			return nil
		}
		mb.Lock()
		mb.metadatas[id] = *md
		mb.Unlock()
		return md
	}
	return &dat
}

func (mb *dsMetadataBook) ClaimedSeq(id peer.ID) (seq methods.SeqNr, ok bool) {
	mb.RLock()
	dat, ok := mb.claims[id]
	mb.RUnlock()
	if !ok {
		n, err := mb.loadClaim(id)
		if err != nil {
			return 0, false
		}
		mb.Lock()
		mb.claims[id] = n
		mb.Unlock()
		return n, true
	}
	return dat, true
}

// RegisterSeqClaim updates the latest supposed seq nr of the peer
func (mb *dsMetadataBook) RegisterSeqClaim(id peer.ID, seq methods.SeqNr) (newer bool) {
	mb.Lock()
	dat, ok := mb.claims[id]
	defer mb.Unlock()
	newer = !ok || dat < seq
	if newer {
		mb.claims[id] = seq
	}
	return
}

// RegisterMetaFetch increments how many times we tried to get the peer metadata
// without satisfying answer, returning the counter.
func (mb *dsMetadataBook) RegisterMetaFetch(id peer.ID) uint64 {
	mb.Lock()
	defer mb.Unlock()
	count, _ := mb.fetches[id]
	count += 1
	mb.fetches[id] = count
	return count
}

// RegisterMetadata updates metadata, if newer than previous. Resetting ongoing fetch counter if it's new enough
func (mb *dsMetadataBook) RegisterMetadata(id peer.ID, md methods.MetaData) (newer bool) {
	mb.Lock()
	defer mb.Unlock()
	dat, ok := mb.metadatas[id]
	newer = !ok || dat.SeqNumber < md.SeqNumber
	if newer {
		// will 0 if no claim
		claimed, _ := mb.claims[id]
		if md.SeqNumber >= claimed {
			// if it is newer or equal to best, we can reset the ongoing fetches
			// TODO: protect against super-fast increasing metadata seq nrs.
			mb.fetches[id] = 0
		}
		mb.metadatas[id] = md
		if md.SeqNumber > claimed {
			mb.claims[id] = md.SeqNumber
		}
	}
	return
}

func (mb *dsMetadataBook) flush() error {
	mb.RLock()
	defer mb.RUnlock()
	var clErr error
	// store all claims to datastore before exiting
	for id, cl := range mb.claims {
		if err := mb.storeClaim(id, cl); err != nil {
			clErr = err
			break
		}
	}
	if clErr != nil {
		return clErr
	}
	// store all metadatas to datastore before exiting
	for id, md := range mb.metadatas {
		if err := mb.storeMetadata(id, &md); err != nil {
			clErr = err
			break
		}
	}
	return clErr
}

func (mb *dsMetadataBook) Close() error {
	return mb.flush()
}
