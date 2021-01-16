package dbs

import (
	"errors"
	"github.com/protolambda/zrnt/eth2/db/blocks"
	"github.com/protolambda/zrnt/eth2/beacon"
	"sync"
)

type BlocksDBID string

type BlocksDBs interface {
	Find(id BlocksDBID) (db blocks.DB, ok bool)
	// Create a new database. An empty path creates a memory DB.
	Create(id BlocksDBID, path string, spec *beacon.Spec) (db blocks.DB, err error)
	Remove(id BlocksDBID) (existed bool)
	List() []BlocksDBID
}

type BlocksDBMap struct {
	// DBID -> DB
	dbs sync.Map
}

func (dbm *BlocksDBMap) Find(id BlocksDBID) (db blocks.DB, ok bool) {
	dbi, ok := dbm.dbs.Load(id)
	if !ok {
		return nil, false
	}
	return dbi.(blocks.DB), true
}

func (dbm *BlocksDBMap) Create(id BlocksDBID, path string, spec *beacon.Spec) (db blocks.DB, err error) {
	var c blocks.DB
	if path == "" {
		c = blocks.NewMemDB(spec)
	} else {
		c = blocks.NewFileDB(spec, path)
	}
	_, alreadyExisted := dbm.dbs.LoadOrStore(id, c)
	if alreadyExisted {
		return nil, errors.New("db already existed")
	}
	return c, nil
}

func (dbm *BlocksDBMap) Remove(id BlocksDBID) (existed bool) {
	_, existed = dbm.dbs.Load(id)
	if existed {
		dbm.dbs.Delete(id)
	}
	return
}

func (dbm *BlocksDBMap) List() (out []BlocksDBID) {
	out = make([]BlocksDBID, 0, 4)
	dbm.dbs.Range(func(key, value interface{}) bool {
		id := key.(BlocksDBID)
		out = append(out, id)
		return true
	})
	return
}
