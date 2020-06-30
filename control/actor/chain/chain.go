package chain

import (
	"fmt"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/chain"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/chain/on"
)

type ChainState struct {
	CurrentChain chain.ChainID
}

type ChainCmd struct {
	*base.Base
	*chain.Chains
	*ChainState
	States sdb.DB
}

// TODO: more chain command ideas:
//  - genesis from eth1

func (c *ChainCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "create":
		cmd = &ChainCreateCmd{Base: c.Base, Chains: c.Chains, States: c.States}
	case "copy":
		cmd = &ChainCopyCmd{Base: c.Base}
	case "switch":
		cmd = &ChainSwitchCmd{Base: c.Base, ChainState: c.ChainState}
	case "rm":
		cmd = &ChainRemoveCmd{Base: c.Base, Chains: c.Chains}
	case "on":
		currentChain, ok := c.Chains.Find(c.ChainState.CurrentChain)
		if !ok {
			return nil, fmt.Errorf("current chain was not found. Use 'chain create' to create chains")
		}
		cmd = &on.ChainOnCmd{Base: c.Base, FullChain: currentChain}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *ChainCmd) Routes() []string {
	return []string{"create", "copy", "switch", "rm", "on"}
}

func (c *ChainCmd) Help() string {
	return "Manage eth2 chains"
}
