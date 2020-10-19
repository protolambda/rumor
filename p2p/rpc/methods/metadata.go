package methods

import (
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/protolambda/zrnt/eth2/beacon"
)

var MetaDataRPCv1 = reqresp.RPCMethod{
	Protocol:                  "/eth2/beacon_chain/req/metadata/1/ssz",
	RequestCodec:              (*reqresp.SSZCodec)(nil), // no request data, just empty bytes.
	ResponseChunkCodec:        reqresp.NewSSZCodec(func() reqresp.SerDes { return new(beacon.MetaData) }, beacon.MetadataByteLen, beacon.MetadataByteLen),
	DefaultResponseChunkCount: 1,
}
