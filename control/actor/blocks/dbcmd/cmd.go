package dbcmd

import (
	"github.com/protolambda/ask"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	"github.com/protolambda/rumor/control/actor/base"
)

type DBCmd struct {
	*base.Base
	bdb.DB
}

// TODO: more blocks command ideas:
//  - download from http source
//  - prune based on chain
//  - automatic upload/export to some place
//  - query blocks by attribute (slot, state root, parent root, eth1 data, etc.)

func (c *DBCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "import":
		cmd = &BlocksImportCmd{Base: c.Base, DB: c.DB}
	case "export":
		cmd = &BlocksExportCmd{Base: c.Base, DB: c.DB}
	case "get":
		cmd = &BlocksGetCmd{Base: c.Base, DB: c.DB}
	case "rm":
		cmd = &BlocksRemoveCmd{Base: c.Base, DB: c.DB}
	case "stats":
		cmd = &BlocksStatsCmd{Base: c.Base, DB: c.DB}
	case "list":
		cmd = &BlocksListCmd{Base: c.Base, DB: c.DB}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *DBCmd) Routes() []string {
	return []string{"import", "export", "get", "rm", "stats", "list"}
}

func (c *DBCmd) Help() string {
	return "Manage eth2 beacon blocks"
}
