package track

import (
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/protolambda/rumor/p2p/types"
	"io"
	"time"
)

// IdentifyBook exposes the peer libp2p-identify info.
// Libp2p stores this data in the misc-data book
type IdentifyBook interface {
	ProtocolVersion(id peer.ID) (string, error)
	UserAgent(id peer.ID) (string, error)
}

type ENRBook interface {
	io.Closer

	UpdateENRMaybe(n *enode.Node) (updated bool, data *types.Eth2Data, attnetbits *types.AttnetBits, err error)
	LatestENR() (n *enode.Node)
}

type StatusBook interface {
	io.Closer

	// Status retrieves the peer status, and may be nil if there is no status
	Status(peer.ID) *methods.Status
	// RegisterStatus updates the status of the peer
	RegisterStatus(peer.ID, methods.Status)
}

type MetadataBook interface {
	io.Closer

	Metadata(peer.ID) *methods.MetaData
	ClaimedSeq(peer.ID) (seq methods.SeqNr, ok bool)
	RegisterSeqClaim(id peer.ID, seq methods.SeqNr) (newer bool)
	RegisterMetaFetch(peer.ID) uint64
	RegisterMetadata(id peer.ID, md methods.MetaData) (newer bool)
}

type PeerAllData struct {
	PeerID peer.ID  `json:"peer_id"`
	NodeID enode.ID `json:"node_id"`
	Pubkey string   `json:"pubkey"`

	Addrs     []ma.Multiaddr `json:"addrs"`
	Protocols []string       `json:"protocols"`

	Latency time.Duration `json:"latency"`

	UserAgent       string `json:"user_agent"`
	ProtocolVersion string `json:"protocol_version"`

	// Metadata with highest sequence number
	MetaData *methods.MetaData `json:"metadata"`
	// Highest claimed seq nr, we may not have the actual corresponding metadata yet.
	ClaimedSeq methods.SeqNr `json:"claimed_seq"`
	// Latest status
	Status *methods.Status `json:"status"`
	// Latest ENR
	ENR *enode.Node `json:"enr"`
}

type AllDataGetter interface {
	GetAllData(id peer.ID) *PeerAllData
}

type ExtendedPeerstore interface {
	peerstore.Peerstore
	StatusBook
	MetadataBook
	ENRBook
	AllDataGetter
	// TODO: maybe track when we've last been connected to a peer?
}
