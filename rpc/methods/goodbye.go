package methods

import (
	"github.com/protolambda/rumor/rpc/reqresp"
	"github.com/protolambda/zssz"
)

type Goodbye uint64

var GoodbyeSSZ = zssz.GetSSZ((*Goodbye)(nil))

var GoodbyeRPCv1 = reqresp.RPCMethod{
	Protocol: "/eth2/beacon_chain/req/goodbye/1/ssz",
	MaxChunkCount: 1,
	ReqSSZ: GoodbyeSSZ,
	RespChunkSSZ: GoodbyeSSZ,
	AllocRequest: func() interface{} {
		return new(Goodbye)
	},
}
