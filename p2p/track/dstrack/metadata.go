package dstrack

import (
	"bytes"
	"encoding/binary"
	"fmt"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/ztyp/codec"
	"sync"
)

var (
	metadataSuffix = ds.NewKey("/metadata")
	claimSuffix    = ds.NewKey("/metadata_claim")
)

type dsMetadataBook struct {
	ds ds.Datastore
	// cache metadata objects to not load/store them all the time
	sync.RWMutex
	// Track metadata with highest sequence number
	metadatas map[peer.ID]beacon.MetaData
	// highest claimed seq nr, we may not have the actual corresponding metadata yet.
	claims map[peer.ID]beacon.SeqNr
	// Track how many times we have tried to ask them for metadata without getting an answer
	fetches map[peer.ID]uint64
}

var _ track.MetadataBook = (*dsMetadataBook)(nil)

func NewMetadataBook(store ds.Datastore) (*dsMetadataBook, error) {
	return &dsMetadataBook{
		ds:        store,
		metadatas: make(map[peer.ID]beacon.MetaData),
		claims:    make(map[peer.ID]beacon.SeqNr),
		fetches:   make(map[peer.ID]uint64),
	}, nil
}

func (mb *dsMetadataBook) loadMetadata(p peer.ID) (*beacon.MetaData, error) {
	key := peerIdToKey(eth2Base, p).Child(metadataSuffix)
	value, err := mb.ds.Get(key)
	if err != nil {
		return nil, fmt.Errorf("error while fetching metadata from datastore for peer %s: %s\n", p.Pretty(), err)
	}
	var md beacon.MetaData
	if err := md.Deserialize(codec.NewDecodingReader(bytes.NewReader(value), uint64(len(value)))); err != nil {
		return nil, fmt.Errorf("failed parse metadata bytes from datastore: %v", err)
	}
	return &md, nil
}

func (mb *dsMetadataBook) storeMetadata(p peer.ID, md *beacon.MetaData) error {
	key := peerIdToKey(eth2Base, p).Child(metadataSuffix)
	size := md.FixedLength()
	out := bytes.NewBuffer(make([]byte, 0, size))
	if err := md.Serialize(codec.NewEncodingWriter(out)); err != nil {
		return fmt.Errorf("failed encode metadata bytes for datastore: %v", err)
	}
	if err := mb.ds.Put(key, out.Bytes()); err != nil {
		return fmt.Errorf("failed to store metadata: %v", err)
	}
	return nil
}

func (mb *dsMetadataBook) loadClaim(p peer.ID) (beacon.SeqNr, error) {
	key := peerIdToKey(eth2Base, p).Child(claimSuffix)
	value, err := mb.ds.Get(key)
	if err != nil {
		return 0, fmt.Errorf("error while fetching claim seq nr from datastore for peer %s: %s\n", p.Pretty(), err)
	}
	claim := beacon.SeqNr(binary.LittleEndian.Uint64(value))
	return claim, nil
}

func (mb *dsMetadataBook) storeClaim(p peer.ID, claim beacon.SeqNr) error {
	key := peerIdToKey(eth2Base, p).Child(claimSuffix)
	var dat [8]byte
	binary.LittleEndian.PutUint64(dat[:], uint64(claim))
	if err := mb.ds.Put(key, dat[:]); err != nil {
		return fmt.Errorf("failed to store claim seq nr: %v", err)
	}
	return nil
}

func (mb *dsMetadataBook) Metadata(id peer.ID) *beacon.MetaData {
	mb.Lock()
	defer mb.Unlock()
	return mb.metadata(id)
}

func (mb *dsMetadataBook) metadata(id peer.ID) *beacon.MetaData {
	dat, ok := mb.metadatas[id]
	if !ok {
		md, err := mb.loadMetadata(id)
		if err != nil {
			return nil
		}
		mb.metadatas[id] = *md
		return md
	}
	return &dat
}

func (mb *dsMetadataBook) ClaimedSeq(id peer.ID) (seq beacon.SeqNr, ok bool) {
	mb.Lock()
	defer mb.Unlock()
	return mb.claimedSeq(id)
}

func (mb *dsMetadataBook) claimedSeq(id peer.ID) (seq beacon.SeqNr, ok bool) {
	dat, ok := mb.claims[id]
	if !ok {
		n, err := mb.loadClaim(id)
		if err != nil {
			return 0, false
		}
		mb.claims[id] = n
		return n, true
	}
	return dat, true
}

// RegisterSeqClaim updates the latest supposed seq nr of the peer
func (mb *dsMetadataBook) RegisterSeqClaim(id peer.ID, seq beacon.SeqNr) (newer bool) {
	mb.Lock()
	defer mb.Unlock()
	dat, ok := mb.claimedSeq(id)
	newer = !ok || dat < seq
	if newer {
		mb.claims[id] = seq
		_ = mb.storeClaim(id, seq)
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
func (mb *dsMetadataBook) RegisterMetadata(id peer.ID, md beacon.MetaData) (newer bool) {
	mb.Lock()
	defer mb.Unlock()
	dat := mb.metadata(id)
	newer = dat == nil || dat.SeqNumber < md.SeqNumber
	if newer {
		// will 0 if no claim
		claimed, _ := mb.claims[id]
		if md.SeqNumber >= claimed {
			// if it is newer or equal to best, we can reset the ongoing fetches
			mb.fetches[id] = 0
		}
		mb.metadatas[id] = md
		_ = mb.storeMetadata(id, &md)
		if md.SeqNumber > claimed {
			mb.claims[id] = md.SeqNumber
			_ = mb.storeClaim(id, md.SeqNumber)
		}
	}
	return
}

func (mb *dsMetadataBook) flush() error {
	mb.RLock()
	defer mb.RUnlock()
	// store all claims to datastore before exiting
	for id, cl := range mb.claims {
		if err := mb.storeClaim(id, cl); err != nil {
			return err
		}
	}
	// store all metadatas to datastore before exiting
	for id, md := range mb.metadatas {
		if err := mb.storeMetadata(id, &md); err != nil {
			return err
		}
	}
	return nil
}

func (mb *dsMetadataBook) Close() error {
	return mb.flush()
}
