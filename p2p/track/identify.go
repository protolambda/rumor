package track

import "github.com/libp2p/go-libp2p-core/peer"

// IdentifyBook exposes the peer libp2p-identify info.
// Libp2p stores this data in the misc-data book
type IdentifyBook interface {
	ProtocolVersion(id peer.ID) (string, error)
	UserAgent(id peer.ID) (string, error)
}
