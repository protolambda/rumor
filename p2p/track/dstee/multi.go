package dstee

import (
	ds "github.com/ipfs/go-datastore"
	"strings"
)

type MultiTee map[Tee]struct{}

func (m MultiTee) String() string {
	var out strings.Builder
	out.WriteString("Multi tee:\n")
	for t := range m {
		out.WriteString(t.String())
		out.WriteRune('\n')
	}
	return out.String()
}

func (m MultiTee) OnPut(key ds.Key, value []byte) {
	for t := range m {
		t.OnPut(key, value)
	}
}

func (m MultiTee) OnDelete(key ds.Key) {
	for t := range m {
		t.OnDelete(key)
	}
}

func (m MultiTee) OnBatch(puts []BatchItem, deletes []ds.Key) {
	for t := range m {
		t.OnBatch(puts, deletes)
	}
}
