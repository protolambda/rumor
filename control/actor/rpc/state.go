package rpc

import (
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"sync"
)

type RPCState struct {
	Goodbye       Responder
	Status        Responder
	Ping          Responder
	Metadata      Responder
	BlocksByRange Responder
	BlocksByRoot  Responder
}

type RequestKey uint64

type RequestEntry struct {
	From    peer.ID
	Handler reqresp.RequestResponder
	Cancel  func()
}

type Responder struct {
	keyCounter      RequestKey
	keyCounterMutex sync.Mutex
	// RequestKey -> RequestEntry
	Requests sync.Map
}

func (r *Responder) GetRequest(key RequestKey) *RequestEntry {
	e, ok := r.Requests.Load(key)
	if ok {
		return e.(*RequestEntry)
	} else {
		return nil
	}
}

func (r *Responder) CloseRequest(key RequestKey) {
	e := r.GetRequest(key)
	if e == nil {
		return
	}
	e.Cancel()
	r.Requests.Delete(key)
}

func (r *Responder) AddRequest(req *RequestEntry) RequestKey {
	r.keyCounterMutex.Lock()
	key := r.keyCounter
	r.keyCounter += 1
	r.keyCounterMutex.Unlock()
	r.Requests.Store(key, req)
	return key
}
