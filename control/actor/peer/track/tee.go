package track

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/protolambda/rumor/p2p/track/dstee"
	"github.com/sirupsen/logrus"
	"os"
	"regexp"
	"strconv"
	"sync"
)

type PeerTrackTeeCmd struct {
	*base.Base
	Store track.ExtendedPeerstore
	Match string `ask:"--match" help:"Datastore key path matcher regex, to select what to track changes of. Empty to match everything."`
	// TODO support http api, db, socket, websocket, anything.
	Dest string `ask:"--dest" help:"Destination to direct events to. Could be: log,csv,evlog,evcsv,json"`
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
		tee = &dstee.LogTee{
			Log: c.Log.WithField("tracker", "log"),
		}
	case "evlog":
		log := c.Log.WithField("tracker", "log")
		tee = &dstee.EventTee{Fn: func(evs ...*dstee.Event) {
			for _, ev := range evs {
				f := logrus.Fields{
					"op":      ev.Op,
					"peer_id": ev.PeerID,
					"time_ms": ev.TimeMs,
				}
				if ev.DelPath != "" {
					f["del_path"] = ev.DelPath
				}
				if ev.Entry != nil {
					f["entry"] = ev.Entry
				}
				log.WithFields(f)
			}
		}}
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
			w.Flush()
		}
		tee = &dstee.CSVTee{
			Name: c.Path,
			CSV:  w,
			Log:  c.Log.WithField("tracker", "csv"),
		}
		clean = func() error {
			w.Flush()
			return f.Close()
		}
	case "evcsv":
		f, err := os.OpenFile(c.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		w := csv.NewWriter(f)
		// Try to write a header if it's a new empty file.
		if st, err := f.Stat(); err == nil {
			if st.Size() == 0 {
				_ = w.Write([]string{"op", "time_ms", "peer_id", "key", "value"})
			}
			w.Flush()
		}
		var l sync.Mutex
		tee = &dstee.EventTee{Fn: func(evs ...*dstee.Event) {
			l.Lock()
			defer l.Unlock()
			for _, ev := range evs {
				if ev.Op == dstee.Delete {
					if err := w.Write([]string{"del", strconv.FormatUint(ev.TimeMs, 10), ev.PeerID.String(), ev.DelPath, ""}); err != nil {
						c.Log.Warn("failed to write event to csv output")
					}
				} else {
					if err := w.WriteAll(ev.Entry.ToCSV(string(ev.Op), strconv.FormatUint(ev.TimeMs, 10), ev.PeerID.String())); err != nil {
						c.Log.Warn("failed to write events to csv output")
					}
				}
			}
			w.Flush()
		}}
		clean = func() error {
			w.Flush()
			return f.Close()
		}
	case "json":
		f, err := os.OpenFile(c.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(f)
		var l sync.Mutex
		tee = &dstee.EventTee{Fn: func(evs ...*dstee.Event) {
			l.Lock()
			defer l.Unlock()
			for _, ev := range evs {
				if err := enc.Encode(ev); err != nil {
					c.Log.Warn("failed to write event to json output")
				}
			}
		}}
		clean = func() error {
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
		tee = &dstee.FilterTee{
			Inner:   tee,
			Pattern: r,
		}
	}
	_ = c.Store.AddTee(tee)
	c.Control.RegisterStop(func(ctx context.Context) error {
		c.Log.Infof("Stopping peer tracker: %s", tee)
		c.Store.RmTee(tee)
		if clean != nil {
			return clean()
		}
		return nil
	})
	return nil
}
