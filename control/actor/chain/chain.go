package chain

import (
	"fmt"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/chain"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/chain/chcmd"
)

type ChainState struct {
	CurrentChain chain.ChainID
}

type ChainCmd struct {
	*base.Base
	chain.Chains
	*ChainState
	Blocks bdb.DB
	States sdb.DB
}

// TODO: more chain command ideas:
//  - genesis from eth1

func (c *ChainCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "create":
		cmd = &ChainCreateCmd{Base: c.Base, Chains: c.Chains, States: c.States, ChainState: c.ChainState}
	case "copy":
		cmd = &ChainCopyCmd{Base: c.Base}
	case "switch":
		cmd = &ChainSwitchCmd{Base: c.Base, ChainState: c.ChainState}
	case "rm":
		cmd = &ChainRemoveCmd{Base: c.Base, Chains: c.Chains}
	case "list":
		cmd = &ChainListCmd{Base: c.Base, Chains: c.Chains, ChainState: c.ChainState}
	case "this":
		currentChain, ok := c.Chains.Find(c.ChainState.CurrentChain)
		if !ok {
			return nil, fmt.Errorf("current chain was not found. Use 'chain create' to create chains")
		}
		cmd = &chcmd.ChainCmd{Base: c.Base, Chain: currentChain, Blocks: c.Blocks, States: c.States}
	case "on":
		cmd = &OnCmd{Base: c.Base, Chains: c.Chains, Blocks: c.Blocks, States: c.States}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *ChainCmd) Routes() []string {
	return []string{"create", "copy", "switch", "rm", "list", "this", "on"}
}

func (c *ChainCmd) Help() string {
	return "Manage eth2 chains"
}
