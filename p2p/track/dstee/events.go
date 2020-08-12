package dstee

import (
	"fmt"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/rumor/p2p/track/dstee/translate"
	"time"
)

type Event struct {
	Op      Operation                        `json:"op"`
	PeerID  peer.ID                          `json:"peer_id"`
	TimeMs  uint64                           `json:"time_ms"`
	DelPath string                           `json:"del_path,omitempty"`
	Entry   *translate.PartialPeerstoreEntry `json:"change,omitempty"`
}

// EventTee transforms raw puts/deletes into events, and forwards them to a function.
// The EventTee wraps the function, to be a hashable type for use in multi-tees.
type EventTee struct {
	Fn func(evs ...*Event)
}

func (t *EventTee) String() string {
	return fmt.Sprintf("Event Tee")
}

func onPut(ms uint64, key ds.Key, value []byte) (*Event, error) {
	id, entry, err := translate.ItemToEntry(key, value)
	if err != nil {
		return nil, err
	} else {
		return &Event{
			Op:     Put,
			PeerID: id,
			TimeMs: ms,
			Entry:  &entry,
		}, nil
	}
}

func (t *EventTee) OnPut(key ds.Key, value []byte) {
	ms := uint64(time.Now().UnixNano() / int64(time.Millisecond))
	if ev, err := onPut(ms, key, value); err != nil {
		// ignore for now
	} else {
		t.Fn(ev)
	}
}

func onDel(ms uint64, key ds.Key) (*Event, error) {
	id, p, err := translate.KeyToPath(key)
	if err != nil {
		return nil, err
	} else {
		return &Event{
			Op:      Delete,
			PeerID:  id,
			TimeMs:  ms,
			DelPath: p,
		}, nil
	}
}

func (t *EventTee) OnDelete(key ds.Key) {
	ms := uint64(time.Now().UnixNano() / int64(time.Millisecond))
	if ev, err := onDel(ms, key); err != nil {
		// ignore for now
	} else {
		t.Fn(ev)
	}
}

func (t *EventTee) OnBatch(puts []BatchItem, deletes []ds.Key) {
	ms := uint64(time.Now().UnixNano() / int64(time.Millisecond))
	evs := make([]*Event, 0, len(puts)+len(deletes))
	for _, p := range puts {
		if ev, err := onPut(ms, p.Key, p.Value); err != nil {
			// ignore for now
		} else {
			evs = append(evs, ev)
		}
	}
	for _, d := range deletes {
		if ev, err := onDel(ms, d); err != nil {
			// ignore for now
		} else {
			evs = append(evs, ev)
		}
	}
	t.Fn(evs...)
}
