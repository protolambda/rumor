package track

import "sync"

type DynamicPeerstore interface {
	ExtendedPeerstore
	// Initialized if the peerstore is ready for use
	Initialized() bool
	// The current ID of the store, may be empty
	PeerstoreID() PeerstoreID
	// Replace the store
	Switch(id PeerstoreID, inner ExtendedPeerstore)
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

func (dp *dynStore) Switch(id PeerstoreID, inner ExtendedPeerstore) {
	dp.replaceLock.Lock()
	defer dp.replaceLock.Unlock()
	dp.currentID = id
	dp.ExtendedPeerstore = inner
}
