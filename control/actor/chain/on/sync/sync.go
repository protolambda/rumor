package sync

import (
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/chain"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	"github.com/protolambda/rumor/control/actor/base"
)

type SyncCmd struct {
	*base.Base
	Chain  chain.FullChain
	Blocks bdb.DB
}

// TODO: implement an auto-sync command, that

func (c *SyncCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "by-range":
		cmd = &ByRangeCmd{Base: c.Base, Chain: c.Chain, Blocks: c.Blocks}
	case "by-root":
		cmd = &ByRootCmd{Base: c.Base, Chain: c.Chain, Blocks: c.Blocks}
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
