package chain

import (
	"errors"
	"github.com/protolambda/rumor/chain"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/chain/chcmd"
)

type OnCmd struct {
	*base.Base
	chain.Chains
	Blocks bdb.DB
	States sdb.DB
}

func (c *OnCmd) Help() string {
	return "Interact with any chain by name"
}

func (c *OnCmd) Routes() (out []string) {
	for _, item := range c.Chains.List() {
		out = append(out, string(item))
	}
	return
}

func (c *OnCmd) Cmd(route string) (cmd interface{}, err error) {
	ch, ok := c.Chains.Find(chain.ChainID(route))
	if !ok {
		return nil, errors.New("chain not available, create one with 'chains create'")
	}
	return &chcmd.ChainCmd{Base: c.Base, Chain: ch, Blocks: c.Blocks, States: c.States}, nil
}
