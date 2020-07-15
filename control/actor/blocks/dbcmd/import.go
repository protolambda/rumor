package dbcmd

import (
	"context"
	"errors"
	"fmt"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	"github.com/protolambda/rumor/control/actor/base"
	"io"
	"os"
)

type BlocksImportCmd struct {
	*base.Base
	bdb.DB
	Input string `ask:"--input" help:"A file path to read the block from as ssz file."`
	Data  []byte `ask:"--data" help:"Alternative to file input, import the block by reading hex-encoded bytes."`
}

func (c *BlocksImportCmd) Help() string {
	return "Import a SignedBeaconBlock"
}

func (c *BlocksImportCmd) Run(ctx context.Context, args ...string) error {
	var r io.Reader
	if c.Input == "" {
		if len(c.Data) == 0 {
			return errors.New("no input data. Try --input or --data to import block from")
		}
	} else {
		f, err := os.OpenFile(c.Input, os.O_RDONLY, os.ModePerm)
		if err != nil {
			return fmt.Errorf("failed to open %s: %v", c.Input, err)
		}
		defer f.Close()
		r = f
	}
	existed, err := c.DB.Import(r)
	if err != nil {
		return err
	}
	c.Log.WithField("existed", existed).Infof("imported block")
	return nil
}
