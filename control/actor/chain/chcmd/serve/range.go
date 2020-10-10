package serve

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/protolambda/rumor/chain"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/protolambda/zrnt/eth2/beacon"
	"time"
)

type ByRangeCmd struct {
	*base.Base

	Blocks bdb.DB
	Chain  chain.FullChain

	Timeout     time.Duration         `ask:"--timeout" help:"Timeout for full request and response. 0 to disable"`
	Compression flags.CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`

	MaxCount uint64 `ask:"--max-count" help:"Max count param in range requests"`
	MaxStep  uint64 `ask:"--max-step" help:"Max step param in range requests"`
}

func (c *ByRangeCmd) Default() {
	c.Timeout = 20 * time.Second
	c.Compression.Compression = reqresp.SnappyCompression{}
	c.MaxCount = 100
	c.MaxStep = 10
}

func (c *ByRangeCmd) Help() string {
	return "Serve the chain by slot range."
}

func (c *ByRangeCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}

	if c.Blocks == nil {
		return errors.New("need a blocks DB to serve blocks from")
	}

	bgCtx, bgCancel := context.WithCancel(context.Background())
	sCtxFn := func() context.Context {
		if c.Timeout == 0 {
			return bgCtx
		}
		reqCtx, _ := context.WithTimeout(bgCtx, c.Timeout)
		return reqCtx
	}
	spec := c.Blocks.Spec()
	method := methods.BlocksByRangeRPCv1(spec)
	prot := method.Protocol
	if c.Compression.Compression != nil {
		prot += protocol.ID("_" + c.Compression.Compression.Name())
	}
	listenReq := func(ctx context.Context, peerId peer.ID, handler reqresp.ChunkedRequestHandler) {
		f := map[string]interface{}{
			"from": peerId.String(),
		}
		respondErr := func(code reqresp.ResponseCode, msg string) {
			if err := handler.WriteErrorChunk(code, msg); err != nil {
				c.Log.WithFields(f).WithError(err).Debugf("failed to respond with %d error to failed request", reqresp.InvalidReqCode)
			}
		}
		var req methods.BlocksByRangeReqV1
		if err := handler.ReadRequest(&req); err != nil {
			c.Log.WithFields(f).WithError(err).Warn("failed to read request")
			respondErr(reqresp.InvalidReqCode, "failed to read request")
			return
		}
		f["req"] = req.Data()
		c.Log.WithFields(f).Debug("Got blocks-by-range request")
		if req.Step == 0 {
			c.Log.WithFields(f).Warn("request has 0 step size")
			respondErr(reqresp.InvalidReqCode, "step must not be 0")
			return
		}
		if uint64(req.Count) > c.MaxCount || uint64(req.Step) > c.MaxStep {
			c.Log.WithFields(f).Warn("request has out of bounds size")
			respondErr(reqresp.InvalidReqCode, "request params out of bounds")
			return
		}
		iter, err := c.Chain.Iter()
		if err != nil {
			c.Log.WithFields(f).WithError(err).Warn("cannot iterate chain")
			respondErr(reqresp.ServerErrCode, "no chain available")
			return
		}
		end := req.StartSlot + beacon.Slot(req.Step*req.Count)
		if req.StartSlot < iter.Start() || (end > iter.End()) {
			c.Log.WithFields(f).Warn("request out of bounds")
			respondErr(reqresp.InvalidReqCode, "request out of bounds")
			return
		}
		for slot := req.StartSlot; slot < end; slot += beacon.Slot(req.Step) {
			entry, err := iter.Entry(slot)
			if err != nil {
				c.Log.WithFields(f).WithError(err).Warn("cannot get entry for slot")
				respondErr(reqresp.ServerErrCode, fmt.Sprintf("cannot get entry for slot %d", slot))
				return
			}
			root := entry.BlockRoot()
			r, size, exists, err := c.Blocks.Stream(root)
			if err != nil {
				c.Log.WithFields(f).WithField("block", hex.EncodeToString(root[:])).WithError(err).Warn("failed to load block")
				respondErr(reqresp.ServerErrCode, fmt.Sprintf("failed to load block %s", root))
				return
			}
			if !exists {
				c.Log.WithFields(f).WithField("block", hex.EncodeToString(root[:])).WithError(err).Warn("failed to find block")
				respondErr(reqresp.ServerErrCode, fmt.Sprintf("failed to find block %s", root))
				return
			}
			if err := handler.StreamResponseChunk(reqresp.SuccessCode, size, r); err != nil {
				c.Log.WithFields(f).WithField("block", hex.EncodeToString(root[:])).WithError(err).Warn("failed to write block")
				return
			}
		}
	}
	streamHandler := method.MakeStreamHandler(sCtxFn, c.Compression.Compression, listenReq)
	h.SetStreamHandler(prot, streamHandler)
	c.Log.WithField("started", true).Infof("Started by-range serving")

	c.Control.RegisterStop(func(ctx context.Context) error {
		bgCancel()
		h.RemoveStreamHandler(prot)
		c.Log.Infof("Stopped by-range serving")
		return nil
	})
	return nil
}
