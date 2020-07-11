package rpc

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/sirupsen/logrus"
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
			Base:          c.Base,
			RpcMethodData: c.RpcMethodData,
			Timeout:       10 * time.Second,
			Compression:   flags.CompressionFlag{Compression: reqresp.SnappyCompression{}},
			MaxChunks:     c.Method.DefaultResponseChunkCount,
			Raw:           false,
		}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

// TODO: it may make sense to just collapse this command route, and only support raw requests.
func (c *RpcMethodReqCmd) Routes() []string {
	return []string{"raw"}
}

type RpcMethodReqRawCmd struct {
	*base.Base
	*RpcMethodData
	Timeout     time.Duration         `ask:"--timeout" help:"Timeout for full request and response. 0 to disable"`
	Compression flags.CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
	MaxChunks   uint64                `ask:"--max-chunks" help:"Max response chunk count, if 0, do not wait for a response at all."`
	Raw         bool                  `ask:"--raw" help:"If chunks should be logged as raw hex-encoded byte strings"`
	PeerID      flags.PeerIDFlag      `ask:"<peer-id>" help:"libp2p Peer-ID to request"`
	Data        []byte                `ask:"<data>" help:"Raw uncompressed hex-encoded request data"`
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

	go func() {
		reqErr := c.Method.RunRequest(reqCtx, sFn, c.PeerID.PeerID, c.Compression.Compression,
			reqresp.RequestBytesInput(c.Data), c.MaxChunks,
			func() error {
				return c.Control.Step(func(ctx context.Context) error {
					c.Log.Debug("made request")
					return nil
				})
			},
			func(chunk reqresp.ChunkedResponseHandler) error {
				return c.Control.Step(func(ctx context.Context) error {
					resultCode := chunk.ResultCode()
					f := logrus.Fields{
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
					c.Log.WithFields(f).Info("Received chunk")
					if resultCode == reqresp.SuccessCode {
						return nil
					} else {
						return fmt.Errorf("got %d reqresp result code", resultCode)
					}
				})
			})
		if reqErr != nil {
			c.Log.WithError(reqErr).Error("failed to make request")
		} else {
			c.Log.Infof("Completed request")
		}
	}()

	c.Control.RegisterStop(func(ctx context.Context) error {
		return nil
	})
	return nil
}
