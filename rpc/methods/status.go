package methods

import (
	"github.com/protolambda/rumor/rpc/reqresp"
	"github.com/protolambda/zssz"
)

type Status struct {
	HeadForkVersion ForkVersion
	FinalizedRoot   Root
	FinalizedEpoch  Epoch
	HeadRoot        Root
	HeadSlot        Slot
}

var StatusSSZ = zssz.GetSSZ((*Status)(nil))

var StatusRPCv1 = reqresp.RPCMethod{
	Protocol: "/eth2/beacon_chain/req/status/1/ssz",
	MaxChunkCount: 1,
	ReqSSZ: StatusSSZ,
	RespChunkSSZ: StatusSSZ,
	AllocRequest: func() interface{} {
		return new(Status)
	},
}
