package methods

import (
	"encoding/hex"
	"fmt"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/protolambda/rumor/p2p/types"
)

type MetaData struct {
	SeqNumber SeqNr
	Attnets   types.AttnetBits
}

func (m *MetaData) Data() map[string]interface{} {
	return map[string]interface{}{
		"seq_number": m.SeqNumber,
		"attnets":    hex.EncodeToString(m.Attnets[:]),
	}
}

func (m *MetaData) String() string {
	return fmt.Sprintf("MetaData(seq: %d, bits: %08b)", m.SeqNumber, m.Attnets)
}

var MetaDataRPCv1 = reqresp.RPCMethod{
	Protocol:                  "/eth2/beacon_chain/req/metadata/1/ssz",
	RequestCodec:              (*reqresp.SSZCodec)(nil), // no request data, just empty bytes.
	ResponseChunkCodec:        reqresp.NewSSZCodec((*MetaData)(nil)),
	DefaultResponseChunkCount: 1,
}
