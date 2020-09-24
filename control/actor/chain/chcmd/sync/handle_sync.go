package sync

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/protolambda/rumor/chain"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/sirupsen/logrus"
)

type SyncFn func(blocksCh chan<- *beacon.SignedBeaconBlock) error

type handleSync struct {
	Log logrus.FieldLogger

	Blocks bdb.DB
	Chain  chain.FullChain

	ExpectedCount uint64

	Store   bool
	Process bool
}

func (c handleSync) handle(processingCtx context.Context, runSync SyncFn) error {
	blocksCh := make(chan *beacon.SignedBeaconBlock, c.ExpectedCount)

	var syncErr error
	go func() {
		// after sync request is done, close the channel and stop sync
		defer close(blocksCh)
		syncErr = runSync(blocksCh)
	}()

	spec := c.Blocks.Spec()
	// TODO: option to buffer batches of blocks, to then process as aggregate.

	i := 0
processLoop:
	for {
		select {
		case block, ok := <-blocksCh:
			if !ok {
				break processLoop
			}
			i += 1
			withRoot := bdb.WithRoot(spec, block)
			if c.Process {
				if err := c.Chain.AddBlock(processingCtx, block); err != nil {
					return fmt.Errorf("failed to process block: %v", err)
				}
				c.Log.WithFields(logrus.Fields{
					"i":    i,
					"slot": block.Message.Slot,
					"root": hex.EncodeToString(withRoot.Root[:]),
				}).Debug("processed block")
			}
			if c.Store {
				exists, err := c.Blocks.Store(processingCtx, withRoot)
				if err != nil {
					return fmt.Errorf("failed to store block: %v", err)
				}
				c.Log.WithFields(logrus.Fields{
					"known": exists,
					"i":     i,
					"slot":  block.Message.Slot,
					"root":  hex.EncodeToString(withRoot.Root[:]),
				}).Debug("stored block")
			}
		case <-processingCtx.Done():
			return fmt.Errorf("block processing stopped early, only processed %d blocks", i)
		}
	}

	return syncErr
}
