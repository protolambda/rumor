package methods

import (
	"encoding/hex"
	"fmt"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
)

// instead of parsing the whole body, we can just leave it as bytes.
type BeaconBlockBodyRaw []byte

func (b *BeaconBlockBodyRaw) Limit() uint64 {
	// just cap block body size at 1 MB
	return 1 << 20
}

type BeaconBlock struct {
	Slot          Slot
	ProposerIndex ValidatorIndex
	ParentRoot    Root
	StateRoot     Root
	Body          BeaconBlockBodyRaw
}

type SignedBeaconBlock struct {
	Message   BeaconBlock
	Signature BLSSignature
}

type BlocksByRangeReqV1 struct {
	StartSlot Slot
	Count     uint64
	Step      uint64
}

func (r *BlocksByRangeReqV1) Data() map[string]interface{} {
	return map[string]interface{}{
		"start_slot": r.StartSlot,
		"count": r.Count,
		"step": r.Step,
	}
}

func (r *BlocksByRangeReqV1) String() string {
	return fmt.Sprintf("%v", *r)
}

var BlocksByRangeRPCv1 = reqresp.RPCMethod{
	Protocol:                  "/eth2/beacon_chain/req/beacon_blocks_by_range/1/ssz",
	RequestCodec:              reqresp.NewSSZCodec((*BlocksByRangeReqV1)(nil)),
	ResponseChunkCodec:        reqresp.NewSSZCodec((*SignedBeaconBlock)(nil)),
	DefaultResponseChunkCount: 20,
}

const MAX_REQUEST_BLOCKS_BY_ROOT = 1024

type BlocksByRootReq []Root

func (*BlocksByRootReq) Limit() uint64 {
	return MAX_REQUEST_BLOCKS_BY_ROOT
}

func (r BlocksByRootReq) Data() []string {
	out := make([]string, len(r), len(r))
	for i := range r {
		out[i] = hex.EncodeToString(r[i][:])
	}
	return out
}

func (r BlocksByRootReq) String() string {
	if len(r) == 0 {
		return "empty blocks-by-root request"
	}
	out := make([]byte, 0, len(r)*66)
	for i, root := range r {
		hex.Encode(out[i*66:], root[:])
		out[(i+1)*66-2] = ','
		out[(i+1)*66-1] = ' '
	}
	return "blocks-by-root requested: " + string(out[:len(out)-1])
}

var BlocksByRootRPCv1 = reqresp.RPCMethod{
	Protocol:                  "/eth2/beacon_chain/req/beacon_blocks_by_root/1/ssz",
	RequestCodec:              reqresp.NewSSZCodec((*BlocksByRootReq)(nil)),
	ResponseChunkCodec:        reqresp.NewSSZCodec((*SignedBeaconBlock)(nil)),
	DefaultResponseChunkCount: 20,
}
