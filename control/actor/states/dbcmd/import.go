package dbcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/ztyp/codec"
	"github.com/protolambda/ztyp/tree"
	"io"
	"os"
)

type StatesImportCmd struct {
	*base.Base
	sdb.DB
	Input string `ask:"--input" help:"A file path to read the state from as ssz file."`
	Data  []byte `ask:"--data" help:"Alternative to file input, import the state by reading hex-encoded bytes."`
}

func (c *StatesImportCmd) Help() string {
	return "Import a BeaconState"
}

func (c *StatesImportCmd) Run(ctx context.Context, args ...string) error {
	var state *beacon.BeaconStateView
	{
		var r io.Reader
		var size uint64
		if c.Input == "" {
			if len(c.Data) == 0 {
				return errors.New("no input data. Try --input or --data to import state from")
			}
			r = bytes.NewReader(c.Data)
			size = uint64(len(c.Data))
		} else {
			f, err := os.OpenFile(c.Input, os.O_RDONLY, os.ModePerm)
			if err != nil {
				return fmt.Errorf("failed to open %s: %v", c.Input, err)
			}
			defer f.Close()
			fInfo, err := f.Stat()
			if err != nil {
				return fmt.Errorf("failed to get file size %s: %v", c.Input, err)
			} else {
				size = uint64(fInfo.Size())
			}
			r = f
		}
		var err error
		state, err = beacon.AsBeaconStateView(c.Spec().BeaconState().Deserialize(codec.NewDecodingReader(r, size)))
		if err != nil {
			return err
		}
	}
	existed, err := c.DB.Store(ctx, state)
	if err != nil {
		return err
	}
	root := state.HashTreeRoot(tree.GetHashFn())
	c.Log.WithField("existed", existed).WithField("root", root.String()).Infof("imported state")
	return nil
}
