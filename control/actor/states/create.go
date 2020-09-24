package states

import (
	"context"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/configs"
)

type CreateCmd struct {
	*base.Base
	sdb.DBs
	*DBState
	Name sdb.DBID `ask:"<name>" help:"The name to give to the created db. Must not exist yet."`
	Path string   `ask:"[path]" help:"The path used for the DB. It will be a memory DB if left empty."`
}

func (c *CreateCmd) Help() string {
	return "Create a new DB"
}

func (c *CreateCmd) Run(ctx context.Context, args ...string) error {
	_, err := c.DBs.Create(c.Name, c.Path, configs.Mainnet) // TODO choose config
	if err != nil {
		return err
	}
	c.DBState.CurrentDB = c.Name
	return nil
}
