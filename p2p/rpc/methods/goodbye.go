package methods

import (
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/protolambda/zrnt/eth2/beacon"
)

var GoodbyeRPCv1 = reqresp.RPCMethod{
	Protocol:                  "/eth2/beacon_chain/req/goodbye/1/ssz",
	RequestCodec:              reqresp.NewSSZCodec(func() reqresp.SerDes { return new(beacon.Goodbye) }, 8, 8),
	ResponseChunkCodec:        reqresp.NewSSZCodec(func() reqresp.SerDes { return new(beacon.Goodbye) }, 8, 8),
	DefaultResponseChunkCount: 0,
}
