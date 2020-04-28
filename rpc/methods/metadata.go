package methods

import (
	"fmt"
	"github.com/protolambda/rumor/rpc/reqresp"
)

// Switch ResponseChunkCodec codec out if you need non-standard attestation subnet count
const ATTESTATION_SUBNET_COUNT = 64

type AttnetBits [(ATTESTATION_SUBNET_COUNT + 7) / 8]byte

func (ab *AttnetBits) BitLen() uint64 {
	return ATTESTATION_SUBNET_COUNT
}

type MetaData struct {
	SeqNumber uint64
	Attnets   AttnetBits
}

func (m MetaData) String() string {
	return fmt.Sprintf("MetaData(seq: %d, bits: %08b)", m.SeqNumber, m.Attnets)
}

var MetaDataRPCv1 = reqresp.RPCMethod{
	Protocol:                  "/eth2/beacon_chain/req/metadata/1/ssz",
	RequestCodec:              (*reqresp.SSZCodec)(nil), // no request data, just empty bytes.
	ResponseChunkCodec:        reqresp.NewSSZCodec((*MetaData)(nil)),
	DefaultResponseChunkCount: 1,
}
