package methods

import (
	"fmt"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
)

type Ping uint64

func (r Ping) String() string {
	return fmt.Sprintf("Ping(%d)", r)
}

type Pong uint64

func (r Pong) String() string {
	return fmt.Sprintf("Pong(%d)", r)
}

var PingRPCv1 = reqresp.RPCMethod{
	Protocol:                  "/eth2/beacon_chain/req/ping/1/ssz",
	RequestCodec:              reqresp.NewSSZCodec((*Ping)(nil)),
	ResponseChunkCodec:        reqresp.NewSSZCodec((*Pong)(nil)),
	DefaultResponseChunkCount: 1,
}
