package methods

import (
	"fmt"
	"github.com/protolambda/rumor/rpc/reqresp"
	"github.com/protolambda/zssz"
)

type Goodbye uint64

func (r Goodbye) String() string {
	return fmt.Sprintf("Goodbye(%d)", r)
}

var GoodbyeSSZ = zssz.GetSSZ((*Goodbye)(nil))

var GoodbyeRPCv1 = reqresp.RPCMethod{
	Protocol: "/eth2/beacon_chain/req/goodbye/1/ssz",
	MaxChunkCount: 1,
	ReqSSZ: GoodbyeSSZ,
	RespChunkSSZ: GoodbyeSSZ,
	AllocRequest: func() reqresp.Request {
		return new(Goodbye)
	},
}
