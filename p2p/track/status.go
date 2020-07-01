package track

import (
	"bytes"
	"fmt"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-base32"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/protolambda/zssz"
	"io"
	"sync"
)

type Status = methods.Status

type StatusBook interface {
	io.Closer
	Flush() error

	// Status retrieves the peer status, and may be nil if there is no status
	Status(peer.ID) *Status
	// RegisterStatus updates the status of the peer
	RegisterStatus(peer.ID, Status)
}

var (
	statusSuffix  = ds.NewKey("/status")
)

type dsStatusBook struct {
	ds ds.Datastore
	// cache status objects to not load/store them all the time
	data sync.Map
}

var _ StatusBook = (*dsStatusBook)(nil)

func NewStatusBook(store ds.Datastore) (*dsStatusBook, error) {
	return &dsStatusBook{ds: store}, nil
}

func (sb *dsStatusBook) loadStatus(p peer.ID) (*Status, error) {
	key := eth2Base.ChildString(base32.RawStdEncoding.EncodeToString([]byte(p))).Child(statusSuffix)
	value, err := sb.ds.Get(key)
	if err != nil {
		return nil, fmt.Errorf("error while fetching privkey from datastore for peer %s: %s\n", p.Pretty(), err)
	}
	var status Status
	if err := zssz.Decode(bytes.NewReader(value), uint64(len(value)), &status, methods.StatusSSZ); err != nil {
		return nil, fmt.Errorf("failed parse status bytes from datastore: %v", err)
	}
	return &status, nil
}

func (sb *dsStatusBook) storeStatus(p peer.ID, st *Status) error {
	key := eth2Base.ChildString(base32.RawStdEncoding.EncodeToString([]byte(p))).Child(statusSuffix)
	statusSize := zssz.SizeOf(st, methods.StatusSSZ)
	out := bytes.NewBuffer(make([]byte, statusSize, statusSize))
	if _, err := zssz.Encode(out, st, methods.StatusSSZ); err != nil {
		return fmt.Errorf("failed encode status bytes for datastore: %v", err)
	}
	if err := sb.ds.Put(key, out.Bytes()); err != nil {
		return fmt.Errorf("failed to store status: %v", err)
	}
	return nil
}

func (sb *dsStatusBook) Status(id peer.ID) *Status {
	dat, loaded := sb.data.Load(id)
	if loaded {
		return dat.(*Status)
	} else {
		// lazy-load status into the db
		st, err := sb.loadStatus(id)
		if err != nil || st == nil {
			return nil
		} else {
			sb.data.Store(id, st)
			return st
		}
	}
}

// TODO: option to remove Status from the DB?

// RegisterStatus updates latest peer status
func (sb *dsStatusBook) RegisterStatus(id peer.ID, st Status) {
	sb.data.Store(id, &st)
	return
}

func (sb *dsStatusBook) Flush() error {
	var clErr error
	// store all statuses to datastore before exiting
	sb.data.Range(func(key, value interface{}) bool {
		id := key.(peer.ID)
		st := value.(*Status)
		if err := sb.storeStatus(id, st); err != nil {
			clErr = err
			return false
		}
		return true
	})
	return clErr
}

func (sb *dsStatusBook) Close() error {
	return sb.Flush()
}
