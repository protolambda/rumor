package track

import (
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"io"
	"sync"
)

type MetaData = methods.MetaData

type MetadataBook interface {
	io.Closer
	Flush() error

	Metadata(peer.ID) *MetaData
	ClaimedSeq(peer.ID) methods.SeqNr
	RegisterSeqClaim(id peer.ID, seq methods.SeqNr) (newer bool)
	RegisterMetaFetch(peer.ID) uint64
	RegisterMetadata(id peer.ID, md MetaData) (newer bool)
}

type dsMetadataBook struct {
	ds ds.Datastore
	// cache metadata objects to not load/store them all the time
	sync.RWMutex
	// Track metadata with highest sequence number
	metadatas map[peer.ID]MetaData
	// highest claimed seq nr, we may not have the actual corresponding metadata yet.
	claims map[peer.ID]methods.SeqNr
	// Track how many times we have tried to ask them for metadata without getting an answer
	fetches map[peer.ID]uint64
}

var _ MetadataBook = (*dsMetadataBook)(nil)

func (mb *dsMetadataBook) Metadata(id peer.ID) *MetaData {
	return pi.md
}

func (mb *dsMetadataBook) ClaimedSeq(id peer.ID) methods.SeqNr {
	return pi.claimedSeq
}

// RegisterSeqClaim updates the latest supposed seq nr of the peer
func (mb *dsMetadataBook) RegisterSeqClaim(id peer.ID, seq methods.SeqNr) (newer bool) {
	pi.Lock()
	defer pi.Unlock()
	newer = pi.claimedSeq < seq
	if newer {
		pi.claimedSeq = seq
	}
	return
}

// RegisterMetaFetch increments how many times we tried to get the peer metadata
// without satisfying answer, returning the counter.
func (mb *dsMetadataBook) RegisterMetaFetch(id peer.ID) uint64 {
	pi.Lock()
	defer pi.Unlock()
	pi.ongoingMetaFetches++
	return pi.ongoingMetaFetches
}

// RegisterMetadata updates metadata, if newer than previous. Resetting ongoing fetch counter if it's new enough
func (mb *dsMetadataBook) RegisterMetadata(id peer.ID, md MetaData) (newer bool) {
	pi.Lock()
	defer pi.Unlock()
	newer = pi.md.SeqNumber < md.SeqNumber
	if newer {
		if md.SeqNumber >= pi.claimedSeq {
			// if it is newer or equal to best, we can reset the ongoing fetches
			pi.ongoingMetaFetches = 0
		}
		pi.md = md
		if pi.md.SeqNumber > pi.claimedSeq {
			pi.claimedSeq = pi.md.SeqNumber
		}
	}
	return
}
