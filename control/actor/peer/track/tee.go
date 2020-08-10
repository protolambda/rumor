package track

import (
	"context"
	"encoding/csv"
	"fmt"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/protolambda/rumor/p2p/track/dstee"
	"os"
	"regexp"
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
		tee = &dstee.LogTee{
			Log: c.Log.WithField("tracker", "log"),
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
		tee = &dstee.CSVTee{
			Name: c.Path,
			CSV:  w,
			Log:  c.Log.WithField("tracker", "csv"),
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
		tee = &dstee.FilterTee{
			Inner:   tee,
			Pattern: r,
		}
	}
	_ = c.Store.AddTee(tee)
	c.Control.RegisterStop(func(ctx context.Context) error {
		c.Store.RmTee(tee)
		if clean != nil {
			return clean()
		}
		return nil
	})
	return nil
}
