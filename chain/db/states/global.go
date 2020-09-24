package states

import (
	"errors"
	"github.com/protolambda/zrnt/eth2/beacon"
	"sync"
)

type DBID string

type DBs interface {
	Find(id DBID) (db DB, ok bool)
	// Create a new database. An empty path creates a memory DB.
	Create(id DBID, path string, spec *beacon.Spec) (db DB, err error)
	Remove(id DBID) (existed bool)
	List() []DBID
}

type DBMap struct {
	// DBID -> DB
	dbs sync.Map
}

func (dbm *DBMap) Find(id DBID) (db DB, ok bool) {
	dbi, ok := dbm.dbs.Load(id)
	if !ok {
		return nil, false
	}
	return dbi.(DB), true
}

func (dbm *DBMap) Create(id DBID, path string, spec *beacon.Spec) (db DB, err error) {
	var c DB
	if path == "" {
		c = &MemDB{spec: spec}
	} else {
		return nil, errors.New("file based DB not yet supported for beacon states")
	}
	_, alreadyExisted := dbm.dbs.LoadOrStore(id, c)
	if alreadyExisted {
		return nil, errors.New("db already existed")
	}
	return c, nil
}

func (dbm *DBMap) Remove(id DBID) (existed bool) {
	_, existed = dbm.dbs.Load(id)
	if existed {
		dbm.dbs.Delete(id)
	}
	return
}

func (dbm *DBMap) List() (out []DBID) {
	out = make([]DBID, 0, 4)
	dbm.dbs.Range(func(key, value interface{}) bool {
		id := key.(DBID)
		out = append(out, id)
		return true
	})
	return
}
