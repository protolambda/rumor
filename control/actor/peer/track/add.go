package track

import (
	"context"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	ds "github.com/ipfs/go-datastore"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/protolambda/rumor/p2p/track/dstee"
	"github.com/sirupsen/logrus"
	"os"
	"strconv"
	"sync"
	"time"
)

type PeerTrackCmd struct {
	*base.Base
	Store track.ExtendedPeerstore
	Path  string `ask:"--path" help:"Datastore path (regex supported) in the peerstore to track changes of."`
	// TODO support http api, db, socket, websocket, anything.
	DestType string `ask:"--dest-type" help:"Destination to direct events to. Could be: log,csv"`
	Dest     string `ask:"--dest" help:"Path, address or other destination uri"`
}

func (c *PeerTrackCmd) Help() string {
	return "Add a tracker"
}

func (c *PeerTrackCmd) Default() {
	c.DestType = "log"
}

func (c *PeerTrackCmd) Run(ctx context.Context, args ...string) error {
	var tee dstee.Tee
	var clean func() error
	switch c.DestType {
	case "log":

	case "csv":
		f, err := os.OpenFile(c.Dest, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		w := csv.NewWriter(f)

		tee = &csvTracker{
			w:     w,
			log:   c.Log.WithField("tracker", "csv"),
			Mutex: sync.Mutex{},
		}

		clean = func() error {
			w.Flush()
			return f.Close()
		}

	default:
		return fmt.Errorf("unrecognized tracker output type: %s", c.DestType)
	}
	_ = c.Store.AddTee(tee)
	c.Control.RegisterStop(func(ctx context.Context) error {
		c.Store.RmTee(tee)
		return clean()
	})
	return nil
}

type csvTracker struct {
	w   *csv.Writer
	log logrus.FieldLogger
	sync.Mutex
}

func (t *csvTracker) OnPut(key ds.Key, value []byte) {
	t.Lock()
	defer t.Unlock()
	if err := t.w.Write([]string{
		"put",
		strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10),
		key.String(),
		hex.EncodeToString(value), // TODO: maybe format some special sub paths, e.g. IPs, utf-8 values, etc.
	}); err != nil {
		t.log.WithError(err).Error("Failed to write put")
	}
}

func (t *csvTracker) OnDelete(key ds.Key) {
	t.Lock()
	defer t.Unlock()
	if err := t.w.Write([]string{
		"del",
		strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10),
		key.String(),
		"",
	}); err != nil {
		t.log.WithError(err).Error("Failed to write delete")
	}
}

func (t *csvTracker) OnBatch(puts []dstee.BatchItem, deletes []ds.Key) {
	t.Lock()
	defer t.Unlock()
	m := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
	for i, p := range puts {
		if err := t.w.Write([]string{
			"put",
			m,
			p.Key.String(),
			hex.EncodeToString(p.Value),
		}); err != nil {
			t.log.WithError(err).WithField("i", i).Error("Failed to write batch put entry")
		}
	}
	for i, d := range deletes {
		if err := t.w.Write([]string{
			"del",
			m,
			d.String(),
			"",
		}); err != nil {
			t.log.WithError(err).WithField("i", i).Error("Failed to write batch delete entry")
		}
	}
}
