package methods

import (
	"encoding/hex"
	"fmt"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/ztyp/codec"
	"github.com/protolambda/ztyp/tree"
	"github.com/protolambda/ztyp/view"
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
	Count     view.Uint64View
	Step      view.Uint64View
}

func (r *BlocksByRangeReqV1) Data() map[string]interface{} {
	return map[string]interface{}{
		"start_slot": r.StartSlot,
		"count":      r.Count,
		"step":       r.Step,
	}
}

func (d *BlocksByRangeReqV1) Deserialize(dr *codec.DecodingReader) error {
	return dr.FixedLenContainer(&d.StartSlot, &d.Count, &d.Step)
}

func (d *BlocksByRangeReqV1) Serialize(w *codec.EncodingWriter) error {
	return w.FixedLenContainer(&d.StartSlot, &d.Count, &d.Step)
}

const blocksByRangeReqByteLen = 8 + 8 + 8

func (d BlocksByRangeReqV1) ByteLength() uint64 {
	return blocksByRangeReqByteLen
}

func (*BlocksByRangeReqV1) FixedLength() uint64 {
	return blocksByRangeReqByteLen
}

func (d *BlocksByRangeReqV1) HashTreeRoot(hFn tree.HashFn) Root {
	return hFn.HashTreeRoot(&d.StartSlot, &d.Count, &d.Step)
}

func (r *BlocksByRangeReqV1) String() string {
	return fmt.Sprintf("%v", *r)
}

func BlocksByRangeRPCv1(spec *beacon.Spec) *reqresp.RPCMethod {
	return &reqresp.RPCMethod{
		Protocol:                  "/eth2/beacon_chain/req/beacon_blocks_by_range/1/ssz",
		RequestCodec:              reqresp.NewSSZCodec(func() reqresp.SerDes { return new(BlocksByRangeReqV1) }, blocksByRangeReqByteLen, blocksByRangeReqByteLen),
		ResponseChunkCodec:        reqresp.NewSSZCodec(func() reqresp.SerDes { return spec.Wrap(new(beacon.SignedBeaconBlock)) }, 0, spec.SignedBeaconBlock().MaxByteLength()),
		DefaultResponseChunkCount: 20,
	}
}

const MAX_REQUEST_BLOCKS_BY_ROOT = 1024

type BlocksByRootReq []Root

func (a *BlocksByRootReq) Deserialize(dr *codec.DecodingReader) error {
	return tree.ReadRootsLimited(dr, (*[]Root)(a), MAX_REQUEST_BLOCKS_BY_ROOT)
}

func (a BlocksByRootReq) Serialize(w *codec.EncodingWriter) error {
	return tree.WriteRoots(w, a)
}

func (a BlocksByRootReq) ByteLength() (out uint64) {
	return uint64(len(a)) * 32
}

func (a *BlocksByRootReq) FixedLength() uint64 {
	return 0 // it's a list, no fixed length
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

func BlocksByRootRPCv1(spec *beacon.Spec) *reqresp.RPCMethod {
	return &reqresp.RPCMethod{
		Protocol:                  "/eth2/beacon_chain/req/beacon_blocks_by_root/1/ssz",
		RequestCodec:              reqresp.NewSSZCodec(func() reqresp.SerDes { return new(BlocksByRootReq) }, 0, 32*MAX_REQUEST_BLOCKS_BY_ROOT),
		ResponseChunkCodec:        reqresp.NewSSZCodec(func() reqresp.SerDes { return spec.Wrap(new(beacon.SignedBeaconBlock)) }, 0, spec.SignedBeaconBlock().MaxByteLength()),
		DefaultResponseChunkCount: 20,
	}
}
