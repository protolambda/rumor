package blocks

import (
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
)

type BlocksCmd struct {
	*base.Base
}

// TODO: more blocks command ideas:
//  - download from http source
//  - prune based on chain
//  - automatic upload/export to some place

func (c *BlocksCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "import":
		cmd = &BlocksImportCmd{Base: c.Base}
	case "export":
		cmd = &BlocksExportCmd{Base: c.Base}
	case "get":
		cmd = &BlocksGetCmd{Base: c.Base}
	case "rm":
		cmd = &BlocksRemoveCmd{Base: c.Base}
	case "stats":
		cmd = &BlocksStatsCmd{Base: c.Base}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *BlocksCmd) Routes() []string {
	return []string{"import", "export", "get", "rm", "stats"}
}

func (c *BlocksCmd) Help() string {
	return "Manage eth2 beacon blocks"
}
