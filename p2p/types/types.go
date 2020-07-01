package types

import (
	"encoding/hex"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/zssz"
)

type Eth2Data struct {
	ForkDigest      beacon.ForkDigest `json:"fork_digest"`
	NextForkVersion beacon.Version `json:"next_fork_version"`
	NextForkEpoch   beacon.Epoch `json:"next_fork_epoch"`
}

var Eth2DataSSZ = zssz.GetSSZ((*Eth2Data)(nil))

const ATTESTATION_SUBNET_COUNT = 64

type AttnetBits [(ATTESTATION_SUBNET_COUNT + 7) / 8]byte

func (ab *AttnetBits) BitLen() uint64 {
	return ATTESTATION_SUBNET_COUNT
}

func (ab *AttnetBits) MarshalJSON() ([]byte, error) {
	return []byte(`"`+hex.EncodeToString(ab[:])+`"`), nil
}

var AttnetBitsSSZ = zssz.GetSSZ((*AttnetBits)(nil))
