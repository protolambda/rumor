package states

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
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
			r = f
		}
		var err error
		state, err = beacon.AsBeaconStateView(beacon.BeaconStateType.Deserialize(r, size))
		if err != nil {
			return err
		}
	}
	existed, err := c.DB.Store(ctx, state)
	if err != nil {
		return err
	}
	c.Log.WithField("existed", existed).Infof("imported state")
	return nil
}
