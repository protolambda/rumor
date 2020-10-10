package chcmd

import (
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/chain"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/chain/chcmd/cold"
	"github.com/protolambda/rumor/control/actor/chain/chcmd/head"
	"github.com/protolambda/rumor/control/actor/chain/chcmd/hot"
	"github.com/protolambda/rumor/control/actor/chain/chcmd/serve"
	"github.com/protolambda/rumor/control/actor/chain/chcmd/sync"
)

type ChainCmd struct {
	*base.Base
	Chain  chain.FullChain
	Blocks bdb.DB
	States sdb.DB
}

func (c *ChainCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "attestation":
		cmd = &AttestationCmd{Base: c.Base}
	case "block":
		cmd = &BlockCmd{Base: c.Base, Chain: c.Chain, Blocks: c.Blocks}
	case "hot":
		cmd = &hot.HotCmd{Base: c.Base}
	case "cold":
		cmd = &cold.ColdCmd{Base: c.Base}
	case "head":
		cmd = &head.HeadCmd{Base: c.Base}
	case "serve":
		cmd = &serve.ServeCmd{Base: c.Base, Chain: c.Chain, Blocks: c.Blocks}
	case "sync":
		cmd = &sync.SyncCmd{Base: c.Base, Chain: c.Chain, Blocks: c.Blocks}
	case "votes":
		cmd = &VotesCmd{Base: c.Base}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *ChainCmd) Routes() []string {
	return []string{"attestation", "block", "hot", "cold", "head", "serve", "sync", "votes"}
}

func (c *ChainCmd) Help() string {
	return "Manage things on a beacon chain (hot part may have forks)"
}
