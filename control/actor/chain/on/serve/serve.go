package serve

import (
	"github.com/protolambda/ask"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	"github.com/protolambda/rumor/control/actor/base"
)

type ServeCmd struct {
	*base.Base
	Blocks bdb.DB
}

func (c *ServeCmd) Cmd(route string) (cmd interface{}, err error) {
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

func (c *ServeCmd) Routes() []string {
	return []string{"by-range", "by-root"}
}

func (c *ServeCmd) Help() string {
	return "Serve the chain to peers"
}
