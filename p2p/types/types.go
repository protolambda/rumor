package types

import (
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/zssz"
)

type Eth2Data struct {
	ForkDigest      beacon.ForkDigest `json:"fork_digest"`
	NextForkVersion beacon.Version    `json:"next_fork_version"`
	NextForkEpoch   beacon.Epoch      `json:"next_fork_epoch"`
}

var Eth2DataSSZ = zssz.GetSSZ((*Eth2Data)(nil))

const ATTESTATION_SUBNET_COUNT = 64

const attnetByteLen = (ATTESTATION_SUBNET_COUNT + 7) / 8

type AttnetBits [attnetByteLen]byte

func (ab *AttnetBits) BitLen() uint64 {
	return ATTESTATION_SUBNET_COUNT
}

func (p AttnetBits) MarshalText() ([]byte, error) {
	return []byte("0x" + hex.EncodeToString(p[:])), nil
}

func (p AttnetBits) String() string {
	return "0x" + hex.EncodeToString(p[:])
}

func (p *AttnetBits) UnmarshalText(text []byte) error {
	if p == nil {
		return errors.New("cannot decode into nil AttnetBits")
	}
	if len(text) >= 2 && text[0] == '0' && (text[1] == 'x' || text[1] == 'X') {
		text = text[2:]
	}
	if len(text) != attnetByteLen*2 {
		return fmt.Errorf("unexpected length string '%s'", string(text))
	}
	_, err := hex.Decode(p[:], text)
	return err
}

var AttnetBitsSSZ = zssz.GetSSZ((*AttnetBits)(nil))
