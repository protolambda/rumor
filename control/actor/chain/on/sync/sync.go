package sync

import (
	"github.com/protolambda/ask"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	"github.com/protolambda/rumor/control/actor/base"
)

type SyncCmd struct {
	*base.Base
	Blocks bdb.DB
}

func (c *SyncCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "by-range":
		cmd = &ByRangeCmd{Base: c.Base}
	case "by-root":
		cmd = &ByRootCmd{Base: c.Base}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *SyncCmd) Routes() []string {
	return []string{"by-range", "by-root"}
}

func (c *SyncCmd) Help() string {
	return "Sync the chain from peers"
}
