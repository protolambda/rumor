package methods

import (
	"fmt"
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

func (s *Status) String() string {
	return fmt.Sprintf("Status(head_fork_version: %x, finalized_root: %x, finalized_epoch: %d, head_root: %x, head_slot: %d)",
		s.HeadForkVersion, s.FinalizedRoot, s.FinalizedEpoch, s.HeadRoot, s.HeadSlot)
}

var StatusSSZ = zssz.GetSSZ((*Status)(nil))

var StatusRPCv1 = reqresp.RPCMethod{
	Protocol: "/eth2/beacon_chain/req/status/1/ssz",
	MaxChunkCount: 1,
	ReqSSZ: StatusSSZ,
	RespChunkSSZ: StatusSSZ,
	AllocRequest: func() reqresp.Request {
		return new(Status)
	},
}
