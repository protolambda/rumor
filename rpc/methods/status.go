package methods

import (
	"fmt"
	"github.com/protolambda/rumor/rpc/reqresp"
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

var StatusRPCv1 = reqresp.RPCMethod{
	Protocol: "/eth2/beacon_chain/req/status/1/ssz",
	RequestCodec: reqresp.NewSSZCodec((*Status)(nil)),
	ResponseChunkCodec: reqresp.NewSSZCodec((*Status)(nil)),
	DefaultResponseChunkCount: 1,
}
