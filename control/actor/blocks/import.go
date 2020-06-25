package blocks

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
)

type BlocksImportCmd struct {
	*base.Base
	Input string `ask:"--input" help:"A file path to read the block from as ssz file."`
	Data  []byte `ask:"--data" help:"Alternative to file input, import the block by reading hex-encoded bytes."`
}

func (c *BlocksImportCmd) Help() string {
	return "Import a SignedBeaconBlock"
}

func (c *BlocksImportCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
