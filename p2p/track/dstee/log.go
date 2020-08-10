package dstee

import (
	"encoding/hex"
	ds "github.com/ipfs/go-datastore"
	"github.com/sirupsen/logrus"
)

type LogTee struct {
	Log logrus.FieldLogger
	// TODO: maybe an option to customize used log level?
}

func (t *LogTee) String() string {
	return "Logger Tee"
}

func (t *LogTee) OnPut(key ds.Key, value []byte) {
	t.Log.WithFields(logrus.Fields{
		"op":    "put",
		"key":   key.String(),
		"value": hex.EncodeToString(value),
	}).Info("put")
}

func (t *LogTee) OnDelete(key ds.Key) {
	t.Log.WithFields(logrus.Fields{
		"op":  "del",
		"key": key.String(),
	}).Info("delete")
}

func (t *LogTee) OnBatch(puts []BatchItem, deletes []ds.Key) {
	for _, p := range puts {
		t.OnPut(p.Key, p.Value)
	}
	for _, d := range deletes {
		t.OnDelete(d)
	}
}
