package track

import (
	"github.com/libp2p/go-libp2p-core/peer"
	"sync"
)

type DynamicPeerstore interface {
	ExtendedPeerstore
	// Initialized if the peerstore is ready for use
	Initialized() bool
	// The current ID of the store, may be empty
	PeerstoreID() PeerstoreID
	// Replace the store.
	// If a previous store is available, the local peer details (incl priv key)
	// for the "retainPeer" are copied to the new store.
	// The "retainPeer" is optional.
	Switch(retainPeer peer.ID, id PeerstoreID, newInner ExtendedPeerstore)
}

// DynamicPeerstore embeds a peerstore, thus implementing the full peerstore interface,
// while being able to replace it dynamically in a running libp2p host.
type dynStore struct {
	replaceLock sync.Mutex
	currentID   PeerstoreID

	// may be nil if the peerstore does not yet exist.
	ExtendedPeerstore
}

func NewDynamicPeerstore() DynamicPeerstore {
	return &dynStore{}
}

func (dp *dynStore) Initialized() bool {
	return dp.ExtendedPeerstore != nil
}

func (dp *dynStore) PeerstoreID() PeerstoreID {
	return dp.currentID
}

func (dp *dynStore) Switch(retainPeer peer.ID, id PeerstoreID, newInner ExtendedPeerstore) {
	dp.replaceLock.Lock()
	defer dp.replaceLock.Unlock()
	if dp.ExtendedPeerstore != nil && retainPeer != "" {
		priv := dp.ExtendedPeerstore.PrivKey(retainPeer)
		if priv != nil {
			_ = newInner.AddPrivKey(retainPeer, priv)
		}
	}
	dp.currentID = id
	dp.ExtendedPeerstore = newInner
}
