package states

import (
	"errors"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/states/dbcmd"
	"github.com/protolambda/rumor/dbs"
)

type OnCmd struct {
	*base.Base
	dbs.StatesDBs
}

func (c *OnCmd) Help() string {
	return "Interact with any state DB by name"
}

func (c *OnCmd) Routes() (out []string) {
	for _, item := range c.StatesDBs.List() {
		out = append(out, string(item))
	}
	return
}

func (c *OnCmd) Cmd(route string) (cmd interface{}, err error) {
	db, ok := c.StatesDBs.Find(dbs.StatesDBID(route))
	if !ok {
		return nil, errors.New("DB not available. Create one with 'states create'")
	}
	return &dbcmd.DBCmd{Base: c.Base, DB: db}, nil
}
