package dstee

import (
	ds "github.com/ipfs/go-datastore"
)

type BatchItem struct {
	key   ds.Key
	value []byte
}

type TeeID string

type MultiTee map[TeeID]Tee

func (m MultiTee) OnPut(key ds.Key, value []byte) {
	for _, t := range m {
		t.OnPut(key, value)
	}
}

func (m MultiTee) OnDelete(key ds.Key) {
	for _, t := range m {
		t.OnDelete(key)
	}
}

func (m MultiTee) OnBatch(puts []BatchItem, deletes []ds.Key) {
	for _, t := range m {
		t.OnBatch(puts, deletes)
	}
}

type Tee interface {
	OnPut(key ds.Key, value []byte)
	OnDelete(key ds.Key)
	OnBatch(puts []BatchItem, deletes []ds.Key)
}

type DSTee struct {
	ds.Batching
	Tee Tee
}

func (t *DSTee) Put(key ds.Key, value []byte) error {
	if err := t.Batching.Put(key, value); err != nil {
		return err
	}
	t.Tee.OnPut(key, value)
	return nil
}

func (t *DSTee) Delete(key ds.Key) error {
	if err := t.Batching.Delete(key); err != nil {
		return err
	}
	t.Tee.OnDelete(key)
	return nil
}

func (t *DSTee) Batch() (ds.Batch, error) {
	b, err := t.Batching.Batch()
	if err != nil {
		return nil, err
	}
	return &DSTeeBatch{Batch: b, Tee: t.Tee}, nil
}

type DSTeeBatch struct {
	Batch   ds.Batch
	Tee     Tee
	puts    []BatchItem
	deletes []ds.Key
}

func (b *DSTeeBatch) Put(key ds.Key, value []byte) error {
	if err := b.Batch.Put(key, value); err != nil {
		return err
	}
	b.puts = append(b.puts, BatchItem{key, value})
	return nil
}

func (b *DSTeeBatch) Delete(key ds.Key) error {
	if err := b.Batch.Delete(key); err != nil {
		return err
	}
	b.deletes = append(b.deletes, key)
	return nil
}

func (b *DSTeeBatch) Commit() error {
	if err := b.Batch.Commit(); err != nil {
		return err
	}
	b.Tee.OnBatch(b.puts, b.deletes)
	return nil
}

func (b *DSTeeBatch) Reset() {
	b.puts = b.puts[:0]
	b.deletes = b.deletes[:0]
}
