package track

import (
	"errors"
	"sync"
)

type PeerstoreID string

type Peerstores interface {
	List() (out []PeerstoreID)
	Find(id PeerstoreID) (pi ExtendedPeerstore, ok bool)
	Create(id PeerstoreID, store ExtendedPeerstore) error
	Remove(id PeerstoreID) (existed bool)
}

type PeerstoresMap struct {
	// PeerstoreID -> ExtendedPeerstore
	stores sync.Map
}

func (cs *PeerstoresMap) List() (out []PeerstoreID) {
	out = make([]PeerstoreID, 0, 4)
	cs.stores.Range(func(key, value interface{}) bool {
		id := key.(PeerstoreID)
		out = append(out, id)
		return true
	})
	return
}

func (cs *PeerstoresMap) Find(id PeerstoreID) (pi ExtendedPeerstore, ok bool) {
	pii, ok := cs.stores.Load(id)
	if !ok {
		return nil, false
	}
	return pii.(ExtendedPeerstore), true
}

func (cs *PeerstoresMap) Create(id PeerstoreID, store ExtendedPeerstore) error {
	_, alreadyExisted := cs.stores.LoadOrStore(id, store)
	if alreadyExisted {
		return errors.New("peerstore already existed")
	}
	return nil
}

func (cs *PeerstoresMap) Remove(id PeerstoreID) (existed bool) {
	_, existed = cs.stores.Load(id)
	if existed {
		cs.stores.Delete(id)
	}
	return
}
