package states

import (
	"errors"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/states/dbcmd"
)

type OnCmd struct {
	*base.Base
	sdb.DBs
}

func (c *OnCmd) Help() string {
	return "Interact with any state DB by name"
}

func (c *OnCmd) Routes() (out []string) {
	for _, item := range c.DBs.List() {
		out = append(out, string(item))
	}
	return
}

func (c *OnCmd) Cmd(route string) (cmd interface{}, err error) {
	db, ok := c.DBs.Find(sdb.DBID(route))
	if !ok {
		return nil, errors.New("DB not available. Create one with 'states create'")
	}
	return &dbcmd.DBCmd{Base: c.Base, DB: db}, nil
}
