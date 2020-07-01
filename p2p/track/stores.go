package track

import (
	"errors"
	"sync"
)

type PeerstoreID string

type Peerstores struct {
	// PeerstoreID -> ExtendedPeerstore
	stores sync.Map
}

func (cs *Peerstores) Find(id PeerstoreID) (pi ExtendedPeerstore, ok bool) {
	pii, ok := cs.stores.Load(id)
	return pii.(ExtendedPeerstore), ok
}

func (cs *Peerstores) Create(id PeerstoreID, store ExtendedPeerstore) error {
	_, alreadyExisted := cs.stores.LoadOrStore(id, store)
	if alreadyExisted {
		return errors.New("peerstore already existed")
	}
	return nil
}

func (cs *Peerstores) Remove(id PeerstoreID) (existed bool) {
	_, existed = cs.stores.Load(id)
	if existed {
		cs.stores.Delete(id)
	}
	return
}
