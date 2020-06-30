package track

import (
	"fmt"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/protolambda/rumor/p2p/types"
	"sync"
)

type MetaData = methods.MetaData
type Status = methods.Status

type PeerInfo struct {
	// Track metadata with highest sequence number
	md MetaData
	// highest claimed seq nr, we may not have the actual corresponding metadata yet.
	claimedSeq methods.SeqNr
	// Track latest status
	status Status
	// Latest ENR eth2 data
	enrEth2 *types.Eth2Data
	// Latest ENR attnets data
	enrAttnets types.AttnetBits
	// Track how many times we have tried to ask them for metadata without getting an answer
	ongoingMetaFetches uint64
	// Track ENR
	n *enode.Node
	// Lock to avoid concurrent modifications from creating inconsistencies
	sync.Mutex
}

// Update the record tracking of the peer,
// return updated=true if the node is new, or it overrides a previously seen node (by higher seq nr).
// and return eth2 and attnet data, if any.
func (pi *PeerInfo) UpdateMaybe(n *enode.Node) (updated bool, data *types.Eth2Data, attnetbits *types.AttnetBits, err error) {
	pi.Lock()
	defer pi.Unlock()
	if pi.n != nil {
		if pi.n.Seq() >= n.Seq() {
			return false, nil, nil, nil
		}
	}
	pi.n = n
	data, attnets, err := handleNewEnr(n)
	return true, data, attnets, err
}

// Latest fetches the latest ENR of the peer, nil if we have none. The returned ENR may not be mutated.
func (pi *PeerInfo) Latest() (n *enode.Node) {
	return pi.n
}

func handleNewEnr(n *enode.Node) (data *types.Eth2Data, attnetbits *types.AttnetBits, err error) {
	var eth2 addrutil.Eth2ENREntry
	if err := n.Load(&eth2); err == nil {
		dat, err := eth2.Eth2Data()
		if err == nil {
			data = dat
		} else {
			return nil, nil, err
		}
	}
	var attnets addrutil.AttnetsENREntry
	if err := n.Load(&attnets); err == nil {
		dat, err := attnets.AttnetBits()
		if err == nil {
			attnetbits = &dat
		} else {
			return nil, nil, err
		}
	}
	return
}

func (pi *PeerInfo) Metadata() MetaData {
	return pi.md
}

func (pi *PeerInfo) ClaimedSeq() methods.SeqNr {
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
func (pi *PeerInfo) RegisterSeqClaim(seq methods.SeqNr) (newer bool) {
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
	ClaimedSeq methods.SeqNr `json:"claimed_seq"`
	// Track latest status
	Status Status `json:"status"`
	// Track how many times we have tried to ask them for metadata without getting an answer
	OngoingMetaFetches uint64 `json:"ongoing_fetches"`
}

type PeerInfoFinder interface {
	Find(id peer.ID) (pi *PeerInfo, loaded bool)
}

type PeerInfos struct {
	// peer.ID -> *PeerInfo
	infos sync.Map
}

// Find looks for a peer info, and creates a new peer info if necessary
func (ps *PeerInfos) Find(id peer.ID) (pi *PeerInfo, loaded bool) {
	pii, loaded := ps.infos.LoadOrStore(id, &PeerInfo{})
	return pii.(*PeerInfo), loaded
}
