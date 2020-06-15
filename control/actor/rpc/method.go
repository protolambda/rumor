package rpc

import (
	"fmt"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"strconv"
	"time"
)

type RpcMethodData struct {
	Name      string
	Responder *base.Responder
	Method    *reqresp.RPCMethod
}

type RpcMethodCmd struct {
	*base.Base
	*RpcMethodData
}

func (c *RpcMethodCmd) Help() string {
	return fmt.Sprintf("Manage %s RPC", c.Name)
}

func (c *RpcMethodCmd)  Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "req":
		cmd = &RpcMethodReqCmd{
			Base: c.Base, RpcMethodData: c.RpcMethodData,
		}
	case "listen":
		cmd = &RpcMethodListenCmd{
			Base: c.Base, RpcMethodData: c.RpcMethodData,
			Timeout:      10 * time.Second,
			Compression:  flags.CompressionFlag{Compression: reqresp.SnappyCompression{}},
			Raw:          false,
			Drop:         c.Method.DefaultResponseChunkCount == 0,
			Read:         true,
		}
	case "resp":
		cmd = &RpcMethodRespCmd{
			Base: c.Base, RpcMethodData: c.RpcMethodData,
		}
	case "close":
		cmd = &RpcMethodCloseCmd{
			Base: c.Base, RpcMethodData: c.RpcMethodData,
		}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *RpcMethodData) checkAndGetReq(reqKeyStr string) (key base.RequestKey, req *base.RequestEntry, err error) {
	reqId, err := strconv.ParseUint(reqKeyStr, 0, 64)
	if err != nil {
		return 0, nil, fmt.Errorf("Could not parse request key '%s'", reqKeyStr)
	}

	key = base.RequestKey(reqId)
	req = c.Responder.GetRequest(key)
	if req == nil {
		return 0, nil, fmt.Errorf("Could not find request corresponding to key '%s'", key)
	}
	return key, req, nil
}

