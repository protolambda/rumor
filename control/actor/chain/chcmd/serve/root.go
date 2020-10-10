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
	"time"
)

type ByRootCmd struct {
	*base.Base

	Blocks bdb.DB
	Chain  chain.FullChain

	Timeout     time.Duration         `ask:"--timeout" help:"Timeout for full request and response. 0 to disable"`
	Compression flags.CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`

	MaxCount   uint64 `ask:"--max-count" help:"Max amount of roots to accept requests of"`
	WithinView bool   `ask:"--within-view" help:"Only allow requests for blocks within view of chain. I.e. either canon cold, or any hot block."`
}

func (c *ByRootCmd) Default() {
	c.Timeout = 20 * time.Second
	c.Compression.Compression = reqresp.SnappyCompression{}
	c.MaxCount = methods.MAX_REQUEST_BLOCKS_BY_ROOT
	c.WithinView = true
}

func (c *ByRootCmd) Help() string {
	return "Serve the chain by block root."
}

func (c *ByRootCmd) Run(ctx context.Context, args ...string) error {
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
	method := methods.BlocksByRootRPCv1(spec)
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
		var req methods.BlocksByRootReq
		if err := handler.ReadRequest(&req); err != nil {
			c.Log.WithFields(f).WithError(err).Warn("failed to read request")
			respondErr(reqresp.InvalidReqCode, "failed to read request")
			return
		}
		f["req"] = req.Data()
		c.Log.WithFields(f).Debug("Got blocks-by-root request")
		if len(req) == 0 {
			c.Log.WithFields(f).Warn("request has 0 roots")
			return
		}
		if uint64(len(req)) > c.MaxCount {
			c.Log.WithFields(f).Warn("request has too many roots")
			respondErr(reqresp.InvalidReqCode, "request has too many roots")
			return
		}
		if c.WithinView {
			for i, root := range req {
				_, err := c.Chain.ByBlockRoot(root)
				if err != nil {
					c.Log.WithFields(f).WithField("root", hex.EncodeToString(root[:])).Warnf("request root %d unavailable", i)
					respondErr(reqresp.InvalidReqCode, "request root unavailable")
					return
				}
			}
		}

		for _, root := range req {
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
	c.Log.WithField("started", true).Infof("Started by-root serving")

	c.Control.RegisterStop(func(ctx context.Context) error {
		bgCancel()
		h.RemoveStreamHandler(prot)
		c.Log.Infof("Stopped by-root serving")
		return nil
	})
	return nil
}
