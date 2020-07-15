package blocks

import (
	"errors"
	"sync"
)

type DBID string

type DBs interface {
	Find(id DBID) (db DB, ok bool)
	// Create a new database. An empty path creates a memory DB.
	Create(id DBID, path string) (db DB, err error)
	Remove(id DBID) (existed bool)
	List() []DBID
}

type DBMap struct {
	// DBID -> DB
	chains sync.Map
}

func (dbm *DBMap) Find(id DBID) (db DB, ok bool) {
	dbi, ok := dbm.chains.Load(id)
	if !ok {
		return nil, false
	}
	return dbi.(DB), true
}

func (dbm *DBMap) Create(id DBID, path string) (db DB, err error) {
	var c DB
	if path == "" {
		c = &MemDB{}
	} else {
		c = &FileDB{Path: path}
	}
	_, alreadyExisted := dbm.chains.LoadOrStore(id, c)
	if alreadyExisted {
		return nil, errors.New("db already existed")
	}
	return c, nil
}

func (dbm *DBMap) Remove(id DBID) (existed bool) {
	_, existed = dbm.chains.Load(id)
	if existed {
		dbm.chains.Delete(id)
	}
	return
}

func (dbm *DBMap) List() (out []DBID) {
	out = make([]DBID, 0, 4)
	dbm.chains.Range(func(key, value interface{}) bool {
		id := key.(DBID)
		out = append(out, id)
		return true
	})
	return
}
