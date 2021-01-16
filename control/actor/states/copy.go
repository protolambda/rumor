package states

import (
	"context"
	"errors"
	"github.com/protolambda/rumor/dbs"
	"github.com/protolambda/rumor/control/actor/base"
)

type CopyCmd struct {
	*base.Base
	Src  dbs.StatesDBID `ask:"<source>" help:"The source, the DB to copy. Must exist."`
	Dest dbs.StatesDBID `ask:"<dest>" help:"The destination, the name of the copy. Must not exist yet."`
}

func (c *CopyCmd) Help() string {
	return "Copy a DB"
}

func (c *CopyCmd) Run(ctx context.Context, args ...string) error {
	return errors.New("copying not implemented yet") // TODO
}
