package rpc

import (
	"context"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
)

type RpcMethodRespCmd struct {
	*base.Base
	*RpcMethodData
}

func (c *RpcMethodRespCmd) Help() string {
	return "Respond to requests"
}

func (c *RpcMethodRespCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "chunk":
		cmd = &RpcMethodRespChunkCmd{
			Base:          c.Base,
			RpcMethodData: c.RpcMethodData,
		}
	case "invalid-request":
		cmd = &RpcMethodRespErrorCmd{
			Base:          c.Base,
			RpcMethodData: c.RpcMethodData,
			Done:          true,
			ResultCode:    reqresp.InvalidReqCode,
		}
	case "server-error":
		cmd = &RpcMethodRespErrorCmd{
			Base:          c.Base,
			RpcMethodData: c.RpcMethodData,
			Done:          true,
			ResultCode:    reqresp.ServerErrCode,
		}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *RpcMethodRespCmd) Routes() []string {
	return []string{"chunk", "invalid-request", "server-error"}
}

type RpcMethodRespChunkCmd struct {
	*base.Base
	*RpcMethodData
}

func (c *RpcMethodRespChunkCmd) Help() string {
	return "Respond a chunk to a request"
}

func (c *RpcMethodRespChunkCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "raw":
		cmd = &RpcMethodRespChunkRawCmd{
			Base:          c.Base,
			RpcMethodData: c.RpcMethodData,
			// if there is one normally only one response chunk, then close it right after by default.
			Done:       c.Method.DefaultResponseChunkCount <= 1,
			ResultCode: reqresp.SuccessCode,
		}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *RpcMethodRespChunkCmd) Routes() []string {
	return []string{"raw"}
}

type RpcMethodRespChunkRawCmd struct {
	*base.Base
	*RpcMethodData
	Done       bool                 `ask:"--done" help:"After writing this chunk, close the response (no more chunks)."`
	ResultCode reqresp.ResponseCode `ask:"--result-code" help:"Customize the chunk result code. (0 = success, 1 = invalid input, 2 = error, 3+ = undefined)"`
	ReqId      string               `ask:"<req-id>" help:"the ID of the request to respond to"`
	Data       []byte               `ask:"<data>" help:"chunk bytes (uncompressed, hex-encoded)"`
}

func (c *RpcMethodRespChunkRawCmd) Help() string {
	return "Respond a raw chunk to a request"
}

func (c *RpcMethodRespChunkRawCmd) Run(ctx context.Context, args ...string) error {
	key, req, err := c.checkAndGetReq(c.ReqId)
	if err != nil {
		return err
	}
	if err := req.Handler.WriteRawResponseChunk(c.ResultCode, c.Data); err != nil {
		return err
	}
	if c.Done {
		c.Responder.CloseRequest(key)
	}
	return nil
}

type RpcMethodRespErrorCmd struct {
	*base.Base
	*RpcMethodData
	Done       bool                 `ask:"--done" help:"After writing this chunk, close the response (no more chunks)."`
	ResultCode reqresp.ResponseCode `ask:"--result-code" help:"Customize the chunk result code. (0 = success, 1 = invalid input, 2 = error, 3+ = undefined)"`
	ReqId      string               `ask:"<req-id>" help:"the ID of the request to respond to"`
	Text       string               `ask:"<text>" help:"error text data"`
}

func (c *RpcMethodRespErrorCmd) Help() string {
	return "Respond a raw chunk to a request"
}

func (c *RpcMethodRespErrorCmd) Run(ctx context.Context, args ...string) error {
	key, req, err := c.checkAndGetReq(c.ReqId)
	if err != nil {
		return err
	}
	if err := req.Handler.WriteErrorChunk(c.ResultCode, c.Text); err != nil {
		return err
	}
	if c.Done {
		c.Responder.CloseRequest(key)
	}
	return nil
}
