package states

import (
	"errors"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/dbs"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/states/dbcmd"
)

type DBState struct {
	CurrentDB dbs.StatesDBID
}

type StatesCmd struct {
	*base.Base
	dbs.StatesDBs
	*DBState
}

func (c *StatesCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "create":
		cmd = &CreateCmd{Base: c.Base, StatesDBs: c.StatesDBs, DBState: c.DBState}
	case "copy":
		cmd = &CopyCmd{Base: c.Base}
	case "switch":
		cmd = &SwitchCmd{Base: c.Base, DBState: c.DBState}
	case "rm":
		cmd = &RemoveCmd{Base: c.Base, StatesDBs: c.StatesDBs}
	case "list":
		cmd = &ListCmd{Base: c.Base, StatesDBs: c.StatesDBs, DBState: c.DBState}
	case "db":
		db, ok := c.StatesDBs.Find(c.CurrentDB)
		if !ok {
			return nil, errors.New("current DB not available. Create one with 'states create'")
		}
		cmd = &dbcmd.DBCmd{Base: c.Base, DB: db}
	case "on":
		cmd = &OnCmd{Base: c.Base, StatesDBs: c.StatesDBs}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *StatesCmd) Routes() []string {
	return []string{"create", "copy", "switch", "rm", "list", "db", "on"}
}

func (c *StatesCmd) Help() string {
	return "Manage and interact with blocks DBs"
}
