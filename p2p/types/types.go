package types

import (
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/zssz"
)

type Eth2Data struct {
	ForkDigest      beacon.ForkDigest
	NextForkVersion beacon.Version
	NextForkEpoch   beacon.Epoch
}

var Eth2DataSSZ = zssz.GetSSZ((*Eth2Data)(nil))

const ATTESTATION_SUBNET_COUNT = 64

type AttnetBits [(ATTESTATION_SUBNET_COUNT + 7) / 8]byte

func (ab *AttnetBits) BitLen() uint64 {
	return ATTESTATION_SUBNET_COUNT
}

var AttnetBitsSSZ = zssz.GetSSZ((*AttnetBits)(nil))
