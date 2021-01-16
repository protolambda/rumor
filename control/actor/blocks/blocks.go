package blocks

import (
	"errors"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/dbs"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/blocks/dbcmd"
)

type DBState struct {
	CurrentDB dbs.BlocksDBID
}

type BlocksCmd struct {
	*base.Base
	dbs.BlocksDBs
	*DBState
}

func (c *BlocksCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "create":
		cmd = &CreateCmd{Base: c.Base, BlocksDBs: c.BlocksDBs, DBState: c.DBState}
	case "copy":
		cmd = &CopyCmd{Base: c.Base}
	case "switch":
		cmd = &SwitchCmd{Base: c.Base, DBState: c.DBState}
	case "rm":
		cmd = &RemoveCmd{Base: c.Base, BlocksDBs: c.BlocksDBs}
	case "list":
		cmd = &ListCmd{Base: c.Base, BlocksDBs: c.BlocksDBs, DBState: c.DBState}
	case "db":
		db, ok := c.BlocksDBs.Find(c.CurrentDB)
		if !ok {
			return nil, errors.New("current DB not available. Create one with 'blocks create'")
		}
		cmd = &dbcmd.DBCmd{Base: c.Base, DB: db}
	case "on":
		cmd = &OnCmd{Base: c.Base, BlocksDBs: c.BlocksDBs}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *BlocksCmd) Routes() []string {
	return []string{"create", "copy", "switch", "rm", "list", "db", "on"}
}

func (c *BlocksCmd) Help() string {
	return "Manage and interact with blocks DBs"
}
