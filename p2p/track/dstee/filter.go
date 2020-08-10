package dstee

import (
	"fmt"
	ds "github.com/ipfs/go-datastore"
	"regexp"
)

type FilterTee struct {
	Inner   Tee
	Pattern *regexp.Regexp
}

func (t *FilterTee) String() string {
	return fmt.Sprintf("Filtered Tee: pattern = '%s' inner = '%s' ", t.Pattern.String(), t.Inner.String())
}

func (t *FilterTee) OnPut(key ds.Key, value []byte) {
	if t.Pattern.MatchString(key.String()) {
		t.Inner.OnPut(key, value)
	}
}

func (t *FilterTee) OnDelete(key ds.Key) {
	if t.Pattern.MatchString(key.String()) {
		t.Inner.OnDelete(key)
	}
}

func (t *FilterTee) OnBatch(puts []BatchItem, deletes []ds.Key) {
	var filteredPuts []BatchItem
	for _, p := range puts {
		if t.Pattern.MatchString(p.Key.String()) {
			filteredPuts = append(filteredPuts, p)
		}
	}
	var filteredDeletes []ds.Key
	for _, d := range deletes {
		if t.Pattern.MatchString(d.String()) {
			filteredDeletes = append(filteredDeletes, d)
		}
	}
	if len(filteredPuts) > 0 || len(filteredDeletes) > 0 {
		t.Inner.OnBatch(filteredPuts, filteredDeletes)
	}
}
