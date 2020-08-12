package dstee

import (
	"encoding/csv"
	"encoding/hex"
	"fmt"
	ds "github.com/ipfs/go-datastore"
	"github.com/sirupsen/logrus"
	"strconv"
	"sync"
	"time"
)

type CSVTee struct {
	Name string
	CSV  *csv.Writer
	Log  logrus.FieldLogger
	sync.Mutex
}

func (t *CSVTee) String() string {
	return fmt.Sprintf("CSV Tee: %s", t.Name)
}

func (t *CSVTee) OnPut(key ds.Key, value []byte) {
	t.Lock()
	defer t.Unlock()
	if err := t.CSV.Write([]string{
		string(Put),
		strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10),
		key.String(),
		hex.EncodeToString(value), // TODO: maybe format some special sub paths, e.g. IPs, utf-8 values, etc.
	}); err != nil {
		t.Log.WithError(err).Error("Failed to write put")
	} else {
		t.CSV.Flush()
	}
}

func (t *CSVTee) OnDelete(key ds.Key) {
	t.Lock()
	defer t.Unlock()
	if err := t.CSV.Write([]string{
		string(Delete),
		strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10),
		key.String(),
		"",
	}); err != nil {
		t.Log.WithError(err).Error("Failed to write delete")
	} else {
		t.CSV.Flush()
	}
}

func (t *CSVTee) OnBatch(puts []BatchItem, deletes []ds.Key) {
	t.Lock()
	defer t.Unlock()
	m := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
	for i, p := range puts {
		if err := t.CSV.Write([]string{
			string(Put),
			m,
			p.Key.String(),
			hex.EncodeToString(p.Value),
		}); err != nil {
			t.Log.WithError(err).WithField("i", i).Error("Failed to write batch put entry")
		}
	}
	for i, d := range deletes {
		if err := t.CSV.Write([]string{
			string(Delete),
			m,
			d.String(),
			"",
		}); err != nil {
			t.Log.WithError(err).WithField("i", i).Error("Failed to write batch delete entry")
		}
	}
	t.CSV.Flush()
}
