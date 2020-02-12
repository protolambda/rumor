package addrutil

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/rlp"
	"net"
	"strings"
)

type Eth2ENREntry []byte

func (eee Eth2ENREntry) ENRKey() string {
	return "eth2"
}

func (eee Eth2ENREntry) String() string {
	return hex.EncodeToString(eee) // TODO: eth2 ENR, parse based on future spec
}

var EnrEntries = map[string]func() (enr.Entry, func() string) {
	"secp256k1": func() (enr.Entry, func() string) {
		res := new(enode.Secp256k1)
		return res, func() string {
			var out [64]byte
			copy(out[:32], res.X.Bytes())
			copy(out[32:], res.Y.Bytes())
			return hex.EncodeToString(out[:])
		}
	},
	"tcp": func() (enr.Entry, func() string) {
		res := new(enr.TCP)
		return res, func() string {
			return fmt.Sprintf("%d", *res)
		}
	},
	"tcp6": func() (enr.Entry, func() string) {
		res := new(enr.TCP6)
		return res, func() string {
			return fmt.Sprintf("%d", *res)
		}
	},
	"udp": func() (enr.Entry, func() string) {
		res := new(enr.UDP)
		return res, func() string {
			return fmt.Sprintf("%d", *res)
		}
	},
	"udp6": func() (enr.Entry, func() string) {
		res := new(enr.UDP6)
		return res, func() string {
			return fmt.Sprintf("%d", *res)
		}
	},
	"id": func() (enr.Entry, func() string) {
		res := new(enr.ID)
		return res, func() string {
			return string(*res)
		}
	},
	"ip": func() (enr.Entry, func() string) {
		res := new(enr.IP)
		return res, func() string {
			return ((*net.IP)(res)).String()
		}
	},
	"ip6": func() (enr.Entry, func() string) {
		res := new(enr.IPv6)
		return res, func() string {
			return ((*net.IP)(res)).String()
		}
	},
	"eth2": func() (enr.Entry, func() string) {
		res := new(Eth2ENREntry)
		return res, func() string {
			return res.String()
		}
	},
}

func ParseEnrBytes(v string) ([]byte, error) {
	if strings.HasPrefix(v, "enr:") {
		v = v[4:]
		if strings.HasPrefix(v, "//") {
			v = v[2:]
		}
	}
	return base64.RawURLEncoding.DecodeString(v)
}

// pair is a key/value pair in a record.
type EnrInternalPair struct {
	K string
	V rlp.RawValue
}

func (p *EnrInternalPair) ValueEntry() (string, error) {
	vEnrEntryFn, ok := EnrEntries[p.K]
	if !ok {
		return "", fmt.Errorf("enr key %s is unknown", p.K)
	}
	res, resToStr := vEnrEntryFn()
	err := rlp.DecodeBytes(p.V, res)
	if err != nil {
		return "", err
	}
	return resToStr(), nil
}

type ENRInternals struct {
	Pairs []EnrInternalPair
}

func ParseEnrInternals(r *enr.Record) ([]EnrInternalPair, error) {
	var buf bytes.Buffer
	if err := r.EncodeRLP(&buf); err != nil {
		return nil, err
	}
	var internals ENRInternals
	if err := rlp.Decode(bytes.NewReader(buf.Bytes()), &internals); err != nil {
		return nil, err
	}
	return internals.Pairs, nil
}

// DecodeRLP implements rlp.Decoder. Decoding doesn't verify the signature.
func (r *ENRInternals) DecodeRLP(s *rlp.Stream) error {
	raw, err := s.Raw()
	if err != nil {
		return err
	}
	// Decode the RLP container.
	s = rlp.NewStream(bytes.NewReader(raw), 0)
	if _, err := s.List(); err != nil {
		return err
	}
	var sig []byte
	if err = s.Decode(&sig); err != nil {
		return err
	}
	var seq uint64
	if err = s.Decode(&seq); err != nil {
		return err
	}
	// The rest of the record contains sorted k/v pairs.
	var prevkey string
	for i := 0; ; i++ {
		var kv EnrInternalPair
		if err := s.Decode(&kv.K); err != nil {
			if err == rlp.EOL {
				break
			}
			return err
		}
		if err := s.Decode(&kv.V); err != nil {
			if err == rlp.EOL {
				return errors.New("record contains incomplete k/v pair")
			}
			return err
		}
		if i > 0 {
			if kv.K == prevkey {
				return errors.New("record contains duplicate key")
			}
			if kv.K < prevkey {
				return errors.New("record key/value pairs are not sorted by key")
			}
		}
		r.Pairs = append(r.Pairs, kv)
		prevkey = kv.K
	}
	return err
}

func ParseEnr(v string) (*enr.Record, error) {
	data, err := ParseEnrBytes(v)
	if err != nil {
		return nil, err
	}
	var record enr.Record
	if err := rlp.Decode(bytes.NewReader(data), &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func EnrToEnode(record *enr.Record, verifySig bool) (*enode.Node, error) {
	idSchemeName := record.IdentityScheme()

	if verifySig {
		if err := record.VerifySignature(enode.ValidSchemes[idSchemeName]); err != nil {
			return nil, err
		}
	}

	return enode.New(enode.ValidSchemes[idSchemeName], record)
}
