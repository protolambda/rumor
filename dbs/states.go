package dbs

import (
	"errors"
	"github.com/protolambda/zrnt/eth2/db/states"
	"github.com/protolambda/zrnt/eth2/beacon"
	"sync"
)

type StatesDBID string

type StatesDBs interface {
	Find(id StatesDBID) (db states.DB, ok bool)
	// Create a new database. An empty path creates a memory DB.
	Create(id StatesDBID, path string, spec *beacon.Spec) (db states.DB, err error)
	Remove(id StatesDBID) (existed bool)
	List() []StatesDBID
}

type StatesDBMap struct {
	// DBID -> DB
	dbs sync.Map
}

func (dbm *StatesDBMap) Find(id StatesDBID) (db states.DB, ok bool) {
	dbi, ok := dbm.dbs.Load(id)
	if !ok {
		return nil, false
	}
	return dbi.(states.DB), true
}

func (dbm *StatesDBMap) Create(id StatesDBID, path string, spec *beacon.Spec) (db states.DB, err error) {
	var c states.DB
	if path == "" {
		c = states.NewMemDB(spec)
	} else {
		return nil, errors.New("file based DB not yet supported for beacon states")
	}
	_, alreadyExisted := dbm.dbs.LoadOrStore(id, c)
	if alreadyExisted {
		return nil, errors.New("db already existed")
	}
	return c, nil
}

func (dbm *StatesDBMap) Remove(id StatesDBID) (existed bool) {
	_, existed = dbm.dbs.Load(id)
	if existed {
		dbm.dbs.Delete(id)
	}
	return
}

func (dbm *StatesDBMap) List() (out []StatesDBID) {
	out = make([]StatesDBID, 0, 4)
	dbm.dbs.Range(func(key, value interface{}) bool {
		id := key.(StatesDBID)
		out = append(out, id)
		return true
	})
	return
}
