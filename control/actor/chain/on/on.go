package on

import (
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/chain"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/chain/on/cold"
	"github.com/protolambda/rumor/control/actor/chain/on/head"
	"github.com/protolambda/rumor/control/actor/chain/on/hot"
	"github.com/protolambda/rumor/control/actor/chain/on/serve"
	"github.com/protolambda/rumor/control/actor/chain/on/sync"
)

type ChainOnCmd struct {
	*base.Base
	Chain chain.FullChain
	Blocks bdb.DB
	States sdb.DB
}

func (c *ChainOnCmd) Cmd(route string) (cmd interface{}, err error) {
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
		cmd = &serve.ServeCmd{Base: c.Base}
	case "sync":
		cmd = &sync.SyncCmd{Base: c.Base, Chain: c.Chain, Blocks: c.Blocks}
	case "votes":
		cmd = &VotesCmd{Base: c.Base}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *ChainOnCmd) Routes() []string {
	return []string{"attestation", "block", "hot", "cold", "head", "serve", "votes"}
}

func (c *ChainOnCmd) Help() string {
	return "Manage things on a beacon chain (hot part may have forks)"
}
