package dbcmd

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
	"io"
	"os"
)

type BlocksExportCmd struct {
	*base.Base
	bdb.DB
	Output    string      `ask:"--output" help:"A file path to export the block to as ssz file. If empty, output to log."`
	BlockRoot beacon.Root `ask:"<root>" help:"Root of the block to get"`
}

func (c *BlocksExportCmd) Help() string {
	return "Export a SignedBeaconBlock by its block root"
}

func (c *BlocksExportCmd) Run(ctx context.Context, args ...string) (err error) {
	var w io.Writer
	if c.Output == "" {
		var buf bytes.Buffer
		w = &buf
		defer func() {
			c.Log.WithField("data", hex.EncodeToString(buf.Bytes())).Info("block")
		}()
	} else {
		f, err := os.OpenFile(c.Output, os.O_CREATE|os.O_WRONLY, os.ModePerm)
		if err != nil {
			return fmt.Errorf("failed to open %s: %v", c.Output, err)
		}
		defer f.Close()
		w = f
	}
	existed, err := c.DB.Export(c.BlockRoot, w)
	if err != nil {
		return err
	}
	c.Log.WithField("existed", existed).Infof("exported block")
	return nil
}
