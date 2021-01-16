package blocks

import (
	"errors"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/blocks/dbcmd"
	"github.com/protolambda/rumor/dbs"
)

type OnCmd struct {
	*base.Base
	dbs.BlocksDBs
}

func (c *OnCmd) Help() string {
	return "Interact with any blocks DB by name"
}

func (c *OnCmd) Routes() (out []string) {
	for _, item := range c.BlocksDBs.List() {
		out = append(out, string(item))
	}
	return
}

func (c *OnCmd) Cmd(route string) (cmd interface{}, err error) {
	db, ok := c.BlocksDBs.Find(dbs.BlocksDBID(route))
	if !ok {
		return nil, errors.New("DB not available. Create one with 'blocks create'")
	}
	return &dbcmd.DBCmd{Base: c.Base, DB: db}, nil
}
