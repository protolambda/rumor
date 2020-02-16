package methods

import (
	"github.com/protolambda/rumor/rpc/reqresp"
	"github.com/protolambda/zssz"
)

// instead of parsing the whole body, we can just leave it as bytes.
type BeaconBlockBodyRaw []byte

func (b *BeaconBlockBodyRaw) Limit() uint64 {
	// just cap block body size at 1 MB
	return 1 << 20
}

type BeaconBlock struct {
	Slot Slot
	ParentRoot Root
	StateRoot Root
	Body BeaconBlockBodyRaw
}

type SignedBeaconBlock struct {
	Message BeaconBlock
	Signature BLSSignature
}
var SignedBeaconBlockSSZ = zssz.GetSSZ((*SignedBeaconBlock)(nil))


type BlocksByRangeReqV1 struct {
	HeadBlockRoot Root // TO be removed in BlocksByRange v2
	StartSlot Slot
	Count uint64
	Step uint64
}

var BlocksByRangeReqV1SSZ = zssz.GetSSZ((*BlocksByRangeReqV1)(nil))

var BlocksByRangeRPCv1 = reqresp.RPCMethod{
	Protocol:      "/eth2/beacon_chain/req/beacon_blocks_by_range/1/ssz",
	MaxChunkCount: 100, // 100 blocks default maximum
	ReqSSZ:        BlocksByRangeReqV1SSZ,
	RespChunkSSZ:  SignedBeaconBlockSSZ,
	AllocRequest: func() interface{} {
		return new(BlocksByRangeReqV1)
	},
}

type BlocksByRangeReqV2 struct {
	StartSlot Slot
	Count uint64
	Step uint64
}

var BlocksByRangeReqV2SSZ = zssz.GetSSZ((*BlocksByRangeReqV2)(nil))

var BlocksByRangeRPCv2 = reqresp.RPCMethod{
	Protocol:      "/eth2/beacon_chain/req/beacon_blocks_by_range/2/ssz",
	MaxChunkCount: 100, // 100 blocks default maximum
	ReqSSZ:        BlocksByRangeReqV2SSZ,
	RespChunkSSZ:  SignedBeaconBlockSSZ,
	AllocRequest: func() interface{} {
		return new(BlocksByRangeReqV2)
	},
}

type BlocksByRootReq []Root

func (*BlocksByRootReq) Limit() uint64 {
	return 100
}

var BlocksByRootReqSSZ = zssz.GetSSZ((*BlocksByRootReq)(nil))

var BlocksByRootRPCv1 = reqresp.RPCMethod{
	Protocol:      "/eth2/beacon_chain/req/beacon_blocks_by_root/1/ssz",
	MaxChunkCount: 100, // 100 blocks default maximum
	ReqSSZ:        BlocksByRootReqSSZ,
	RespChunkSSZ:  SignedBeaconBlockSSZ,
	AllocRequest: func() interface{} {
		return new(BlocksByRootReq)
	},
}
