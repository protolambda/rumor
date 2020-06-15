package rpc

import (
	"context"
	"encoding/hex"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"time"
)

type RpcMethodReqCmd struct {
	*base.Base
	*RpcMethodData
}

func (c *RpcMethodReqCmd) Help() string {
	return "Make requests"
}

func (c *RpcMethodReqCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "raw":
		cmd = &RpcMethodReqRawCmd{
			Base: c.Base,
			RpcMethodData: c.RpcMethodData,
			Timeout:         10 * time.Second,
			Compression:     flags.CompressionFlag{Compression: reqresp.SnappyCompression{}},
			MaxChunks:       c.Method.DefaultResponseChunkCount,
			Raw:             false,
		}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

type RpcMethodReqRawCmd struct {
	*base.Base
	*RpcMethodData
	Timeout          time.Duration   `ask:"--timeout" help:"Timeout for full request and response. 0 to disable"`
	Compression      flags.CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
	MaxChunks        uint64          `ask:"--max-chunks" help:"Max response chunk count, if 0, do not wait for a response at all."`
	Raw              bool            `ask:"--raw" help:"If chunks should be logged as raw hex-encoded byte strings"`
	PeerID           flags.PeerIDFlag      `ask:"<peer-id>" help:"libp2p Peer-ID to request"`
	Data             flags.BytesHexFlag    `ask:"<data>" help:"Raw uncompressed hex-encoded request data"`
}

func (c *RpcMethodReqRawCmd) Help() string {
	return "Make raw requests"
}

func (c *RpcMethodReqRawCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	sFn := reqresp.NewStreamFn(h.NewStream)

	reqCtx := ctx
	if c.Timeout != 0 {
		reqCtx, _ = context.WithTimeout(reqCtx, c.Timeout)
	}

	protocolId := c.Method.Protocol
	if c.Compression.Compression != nil {
		protocolId += protocol.ID("_" + c.Compression.Compression.Name())
	}
	return c.Method.RunRequest(reqCtx, sFn, c.PeerID.PeerID, c.Compression.Compression,
		reqresp.RequestBytesInput(c.Data), c.MaxChunks,
		func(chunk reqresp.ChunkedResponseHandler) error {
			resultCode := chunk.ResultCode()
			f := map[string]interface{}{
				"protocol":    protocolId,
				"from":        c.PeerID.PeerID.String(),
				"chunk_index": chunk.ChunkIndex(),
				"chunk_size":  chunk.ChunkSize(),
				"result_code": resultCode,
			}
			if c.Raw {
				bytez, err := chunk.ReadRaw()
				if err != nil {
					return err
				}
				f["data"] = hex.EncodeToString(bytez)
			} else {
				switch resultCode {
				case reqresp.ServerErrCode, reqresp.InvalidReqCode:
					msg, err := chunk.ReadErrMsg()
					if err != nil {
						return err
					}
					f["msg"] = msg
				case reqresp.SuccessCode:
					data := c.Method.ResponseChunkCodec.Alloc()
					if err := chunk.ReadObj(data); err != nil {
						return err
					}
					f["data"] = data
				default:
					bytez, err := chunk.ReadRaw()
					if err != nil {
						return err
					}
					f["data"] = hex.EncodeToString(bytez)
				}
			}
			c.Log.WithField("chunk", f).Info("Received chunk")
			return nil
		})
}

