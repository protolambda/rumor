package rpc

import (
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/protolambda/zrnt/eth2/configs"
)

type RpcCmd struct {
	*base.Base
	*RPCState
}

func (c *RpcCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "goodbye":
		cmd = c.Method("goodbye", &c.RPCState.Goodbye, &methods.GoodbyeRPCv1)
	case "status":
		cmd = c.Method("status", &c.RPCState.Status, &methods.StatusRPCv1)
	case "ping":
		cmd = c.Method("ping", &c.RPCState.Ping, &methods.PingRPCv1)
	case "metadata":
		cmd = c.Method("metadata", &c.RPCState.Metadata, &methods.MetaDataRPCv1)
	case "blocks-by-range":
		cmd = c.Method("blocks-by-range", &c.RPCState.BlocksByRange, methods.BlocksByRangeRPCv1(configs.Mainnet)) // TODO configure spec
	case "blocks-by-root":
		cmd = c.Method("blocks-by-root", &c.RPCState.BlocksByRoot, methods.BlocksByRootRPCv1(configs.Mainnet))
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *RpcCmd) Routes() []string {
	return []string{"goodbye", "status", "ping", "metadata", "blocks-by-range", "blocks-by-root"}
}

func (c *RpcCmd) Help() string {
	return "Manage Eth2 RPC"
}

func (c *RpcCmd) Method(name string, resp *Responder, method *reqresp.RPCMethod) *RpcMethodCmd {
	return &RpcMethodCmd{
		Base: c.Base,
		RpcMethodData: &RpcMethodData{
			Name:      name,
			Responder: resp,
			Method:    method,
		}}
}
