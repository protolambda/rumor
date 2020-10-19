package methods

import (
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/protolambda/zrnt/eth2/beacon"
)

var PingRPCv1 = reqresp.RPCMethod{
	Protocol:                  "/eth2/beacon_chain/req/ping/1/ssz",
	RequestCodec:              reqresp.NewSSZCodec(func() reqresp.SerDes { return new(beacon.Ping) }, 8, 8),
	ResponseChunkCodec:        reqresp.NewSSZCodec(func() reqresp.SerDes { return new(beacon.Ping) }, 8, 8),
	DefaultResponseChunkCount: 1,
}
