package track

import (
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	methods2 "github.com/protolambda/rumor/p2p/rpc/methods"
	"sync"
)

type MetaData = methods2.MetaData
type Status = methods2.Status

type PeerInfo struct {
	// Track metadata with highest sequence number
	md MetaData
	// highest claimed seq nr, we may not have the actual corresponding metadata yet.
	claimedSeq uint64
	// Track latest status
	status Status
	// Track how many times we have tried to ask them for metadata without getting an answer
	ongoingMetaFetches uint64
	// Lock to avoid concurrent modifications from creating inconsistencies
	sync.Mutex
}

func (pi *PeerInfo) Metadata() MetaData {
	return pi.md
}

func (pi *PeerInfo) ClaimedSeq() uint64 {
	return pi.claimedSeq
}

func (pi *PeerInfo) Status() Status {
	return pi.status
}

func (pi *PeerInfo) String() string {
	return fmt.Sprintf("info:(meta: %s, claim: %d, status: %s, fetches: %d)",
		pi.md.String(), pi.claimedSeq, pi.status.String(), pi.ongoingMetaFetches)
}

// RegisterSeqClaim updates the latest supposed seq nr of the peer
func (pi *PeerInfo) RegisterSeqClaim(seq uint64) (newer bool) {
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
func (pi *PeerInfo) RegisterMetaFetch() uint64 {
	pi.Lock()
	defer pi.Unlock()
	pi.ongoingMetaFetches++
	return pi.ongoingMetaFetches
}

// RegisterMetadata updates metadata, if newer than previous. Resetting ongoing fetch counter if it's new enough
func (pi *PeerInfo) RegisterMetadata(md MetaData) (newer bool) {
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

// RegisterStatus updates latest peer status
func (pi *PeerInfo) RegisterStatus(st Status) {
	pi.Lock()
	defer pi.Unlock()
	pi.status = st
	return
}

// Data gets a fresh copy of the peer info, encodeable to json and ready for read-only usage
func (pi *PeerInfo) Data(id peer.ID) *PeerInfoData {
	pi.Lock()
	defer pi.Unlock()
	return &PeerInfoData{
		ID:                 id,
		MetaData:           pi.md,
		ClaimedSeq:         pi.claimedSeq,
		Status:             pi.status,
		OngoingMetaFetches: pi.ongoingMetaFetches,
	}
}

type PeerInfoData struct {
	ID peer.ID `json:"id"`
	// Track metadata with highest sequence number
	MetaData MetaData `json:"metadata"`
	// highest claimed seq nr, we may not have the actual corresponding metadata yet.
	ClaimedSeq uint64 `json:"claimed_seq"`
	// Track latest status
	Status Status `json:"status"`
	// Track how many times we have tried to ask them for metadata without getting an answer
	OngoingMetaFetches uint64 `json:"ongoing_fetches"`
}

type PeerInfos struct {
	// peer.ID -> *PeerInfo
	infos sync.Map
}

// Find looks for a peer info, and creates a new peer info if necessary
func (ps *PeerInfos) Find(id peer.ID) (pi *PeerInfo, ok bool) {
	pii, loaded := ps.infos.LoadOrStore(id, &PeerInfo{})
	return pii.(*PeerInfo), loaded
}
