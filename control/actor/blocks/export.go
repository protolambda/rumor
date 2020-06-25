package blocks

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
)

type BlocksExportCmd struct {
	*base.Base
	Output    string      `ask:"--output" help:"A file path to export the block too as ssz file. If empty, output to log."`
	BlockRoot beacon.Root `ask:"<root>" help:"Root of the block to get"`
}

func (c *BlocksExportCmd) Help() string {
	return "Export a SignedBeaconBlock by its block root"
}

func (c *BlocksExportCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
