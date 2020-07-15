package blocks

import (
	"errors"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/blocks/dbcmd"
)

type OnCmd struct {
	*base.Base
	bdb.DBs
}

func (c *OnCmd) Help() string {
	return "Interact with any blocks DB by name"
}

func (c *OnCmd) Routes() (out []string) {
	for _, item := range c.DBs.List() {
		out = append(out, string(item))
	}
	return
}

func (c *OnCmd) Cmd(route string) (cmd interface{}, err error) {
	db, ok := c.DBs.Find(bdb.DBID(route))
	if !ok {
		return nil, errors.New("DB not available. Create one with 'blocks create'")
	}
	return &dbcmd.DBCmd{Base: c.Base, DB: db}, nil
}
