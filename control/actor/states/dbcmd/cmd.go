package dbcmd

import (
	"github.com/protolambda/ask"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
)

type DBCmd struct {
	*base.Base
	sdb.DB
}

// TODO: more States command ideas:
//  - download from http source
//  - prune based on chain
//  - automatic upload/export to some place
//  - query States by attribute (slot, state root, parent root, eth1 data, etc.)

func (c *DBCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "import":
		cmd = &StatesImportCmd{Base: c.Base, DB: c.DB}
	case "export":
		cmd = &StatesExportCmd{Base: c.Base, DB: c.DB}
	case "get":
		cmd = &StatesGetCmd{Base: c.Base, DB: c.DB}
	case "rm":
		cmd = &StatesRemoveCmd{Base: c.Base, DB: c.DB}
	case "stats":
		cmd = &StatesStatsCmd{Base: c.Base, DB: c.DB}
	case "list":
		cmd = &StatesListCmd{Base: c.Base, DB: c.DB}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *DBCmd) Routes() []string {
	return []string{"import", "export", "get", "rm", "stats", "list"}
}

func (c *DBCmd) Help() string {
	return "Manage eth2 beacon States"
}
