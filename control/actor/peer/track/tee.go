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
	"regexp"
	"strconv"
	"sync"
	"time"
)

type PeerTrackTeeCmd struct {
	*base.Base
	Store track.ExtendedPeerstore
	Match string `ask:"--match" help:"Datastore key path matcher regex, to select what to track changes of. Empty to match everything."`
	// TODO support http api, db, socket, websocket, anything.
	Dest string `ask:"--dest" help:"Destination to direct events to. Could be: log,csv"`
	Path string `ask:"--path" help:"Path, address or other destination uri"`
}

func (c *PeerTrackTeeCmd) Help() string {
	return "Track a part of the peerstore and tee it elsewhere"
}

func (c *PeerTrackTeeCmd) Default() {
	c.Dest = "log"
	c.Match = ""
}

func (c *PeerTrackTeeCmd) Run(ctx context.Context, args ...string) error {
	var tee dstee.Tee
	var clean func() error
	switch c.Dest {
	case "log":
		tee = &logTracker{
			log: c.Log.WithField("tracker", "log"),
		}
	case "csv":
		f, err := os.OpenFile(c.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		w := csv.NewWriter(f)
		// Try to write a header if it's a new empty file.
		if st, err := f.Stat(); err == nil {
			if st.Size() == 0 {
				_ = w.Write([]string{"op", "time_ms", "key", "value"})
			}
		}

		tee = &csvTracker{
			w:   w,
			log: c.Log.WithField("tracker", "csv"),
		}

		clean = func() error {
			w.Flush()
			return f.Close()
		}
	default:
		return fmt.Errorf("unrecognized tracker output type: %s", c.Dest)
	}
	if c.Match != "" {
		r, err := regexp.Compile(c.Match)
		if err != nil {
			return fmt.Errorf("failed to parse matcher pattern: %v", err)
		}
		tee = &filterTracker{
			inner:   tee,
			pattern: r,
		}
	}
	_ = c.Store.AddTee(tee)
	c.Control.RegisterStop(func(ctx context.Context) error {
		c.Store.RmTee(tee)
		return clean()
	})
	return nil
}

type filterTracker struct {
	inner   dstee.Tee
	pattern *regexp.Regexp
}

func (t *filterTracker) String() string {
	return fmt.Sprintf("Filtered Tee: pattern = '%s' inner = '%s' ", t.pattern.String(), t.inner.String())
}

func (t *filterTracker) OnPut(key ds.Key, value []byte) {
	if t.pattern.MatchString(key.String()) {
		t.inner.OnPut(key, value)
	}
}

func (t *filterTracker) OnDelete(key ds.Key) {
	if t.pattern.MatchString(key.String()) {
		t.inner.OnDelete(key)
	}
}

func (t *filterTracker) OnBatch(puts []dstee.BatchItem, deletes []ds.Key) {
	var filteredPuts []dstee.BatchItem
	for _, p := range puts {
		if t.pattern.MatchString(p.Key.String()) {
			filteredPuts = append(filteredPuts, p)
		}
	}
	var filteredDeletes []ds.Key
	for _, d := range deletes {
		if t.pattern.MatchString(d.String()) {
			filteredDeletes = append(filteredDeletes, d)
		}
	}
	if len(filteredPuts) > 0 || len(filteredDeletes) > 0 {
		t.inner.OnBatch(filteredPuts, filteredDeletes)
	}
}

type logTracker struct {
	log logrus.FieldLogger
}

func (t *logTracker) String() string {
	return fmt.Sprintf("Logger Tee")
}

func (t *logTracker) OnPut(key ds.Key, value []byte) {
	t.log.WithFields(logrus.Fields{
		"op":    "put",
		"key":   key.String(),
		"value": hex.EncodeToString(value),
	}).Info("put")
}

func (t *logTracker) OnDelete(key ds.Key) {
	t.log.WithFields(logrus.Fields{
		"op":  "del",
		"key": key.String(),
	}).Info("delete")
}

func (t *logTracker) OnBatch(puts []dstee.BatchItem, deletes []ds.Key) {
	for _, p := range puts {
		t.OnPut(p.Key, p.Value)
	}
	for _, d := range deletes {
		t.OnDelete(d)
	}
}

type csvTracker struct {
	outPath string
	w       *csv.Writer
	log     logrus.FieldLogger
	sync.Mutex
}

func (t *csvTracker) String() string {
	return fmt.Sprintf("CSV Tee: %s", t.outPath)
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
