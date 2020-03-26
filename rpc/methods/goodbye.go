package methods

import (
	"fmt"
	"github.com/protolambda/rumor/rpc/reqresp"
)

type Goodbye uint64

func (r Goodbye) String() string {
	return fmt.Sprintf("Goodbye(%d)", r)
}

var GoodbyeRPCv1 = reqresp.RPCMethod{
	Protocol:                  "/eth2/beacon_chain/req/goodbye/1/ssz",
	RequestCodec:              reqresp.NewSSZCodec((*Goodbye)(nil)),
	ResponseChunkCodec:        reqresp.NewSSZCodec((*Goodbye)(nil)),
	DefaultResponseChunkCount: 0,
}
