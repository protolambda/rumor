package dbcmd

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/ztyp/codec"
	"io"
	"os"
)

type StatesExportCmd struct {
	*base.Base
	sdb.DB
	Output    string      `ask:"--output" help:"A file path to export the state to as ssz file. If empty, output to log."`
	StateRoot beacon.Root `ask:"<root>" help:"Root of the state to get"`
}

func (c *StatesExportCmd) Help() string {
	return "Export a BeaconState by its state root"
}

func (c *StatesExportCmd) Run(ctx context.Context, args ...string) (err error) {
	state, exists, err := c.DB.Get(c.StateRoot)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("state %s does not exist", c.StateRoot)
	}
	var w io.Writer
	if c.Output == "" {
		var buf bytes.Buffer
		w = &buf
		defer func() {
			c.Log.WithField("data", hex.EncodeToString(buf.Bytes())).Info("state")
		}()
	} else {
		f, err := os.OpenFile(c.Output, os.O_CREATE|os.O_WRONLY, os.ModePerm)
		if err != nil {
			return fmt.Errorf("failed to open %s: %v", c.Output, err)
		}
		defer f.Close()
		w = f
	}
	if err := state.Serialize(codec.NewEncodingWriter(w)); err != nil {
		return fmt.Errorf("failed to serialize state: %v", err)
	}
	c.Log.Infof("exported state")
	return nil
}
