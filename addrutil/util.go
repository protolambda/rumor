package addrutil

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/btcec"
	"github.com/ethereum/go-ethereum/common/math"
	geth_crypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
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
			peerID := PeerIDFromPubkey((*ecdsa.PublicKey)(res))
			nodeID := NodeIDFromPubkey((*ecdsa.PublicKey)(res))
			return fmt.Sprintf("XY: %x NodeID: %s PeerID: %s", out, nodeID.String(), peerID.Pretty())
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

func ParseEnodeAddr(v string) (*enode.Node, error) {
	if strings.HasPrefix(v, "enode://") {
		addr := new(enode.Node)
		err := addr.UnmarshalText([]byte(v))
		if err != nil {
			return nil, err
		}
		return addr, nil
	} else {
		enrAddr, err := ParseEnr(v)
		if err != nil {
			return nil, err
		}
		// TODO: warn if no "eth2" key in ENR record.
		enodeAddr, err := EnrToEnode(enrAddr, true)
		if err != nil {
			return nil, err
		}
		return enodeAddr, nil
	}
}

func ParsePrivateKey(v string) (*ecdsa.PrivateKey, error) {
	if strings.HasPrefix(v, "0x") {
		v = v[2:]
	}
	privKeyBytes, err := hex.DecodeString(v)
	if err != nil {
		return nil, fmt.Errorf("cannot parse private key, expected hex string: %v", err)
	}
	var priv crypto.PrivKey
	priv, err = crypto.UnmarshalSecp256k1PrivateKey(privKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("cannot parse private key, invalid private key (Secp256k1): %v", err)
	}
	return (*ecdsa.PrivateKey)((priv).(*crypto.Secp256k1PrivateKey)), nil
}

func ParsePubkey(v string) (*ecdsa.PublicKey, error) {
	if strings.HasPrefix(v, "0x") {
		v = v[2:]
	}
	pubKeyBytes, err := hex.DecodeString(v)
	if err != nil {
		return nil, fmt.Errorf("cannot parse public key, expected hex string: %v", err)
	}
	var pub crypto.PubKey
	pub, err = crypto.UnmarshalSecp256k1PublicKey(pubKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("cannot parse public key, invalid public key (Secp256k1): %v", err)
	}
	return (*ecdsa.PublicKey)((pub).(*crypto.Secp256k1PublicKey)), nil
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

func EnrToString(record *enr.Record) (string, error) {
	enc, err := rlp.EncodeToBytes(record)
	if err != nil {
		return "", err
	}
	b64 := base64.RawURLEncoding.EncodeToString(enc)
	return "enr:" + b64, nil
}

func EnodesToMultiAddrs(nodes []*enode.Node) ([]ma.Multiaddr, error) {
	var out []ma.Multiaddr
	for _, n := range nodes {
		if n.IP() == nil {
			continue
		}
		multiAddr, err := EnodeToMultiAddr(n)
		if err != nil {
			return nil, err
		}
		out = append(out, multiAddr)
	}
	return out, nil
}

func PeerIDFromPubkey(pubkey *ecdsa.PublicKey) peer.ID {
	// save for this kind of pubkey
	id, _ := peer.IDFromPublicKey(crypto.PubKey((*crypto.Secp256k1PublicKey)((*btcec.PublicKey)(pubkey))))
	return id
}

func NodeIDFromPubkey(pubkey *ecdsa.PublicKey) enode.ID {
	buf := make([]byte, 64)
	math.ReadBits(pubkey.X, buf[:32])
	math.ReadBits(pubkey.Y, buf[32:])
	var id enode.ID
	copy(id[:], geth_crypto.Keccak256(buf))
	return id
}

func EnodeToMultiAddr(node *enode.Node) (ma.Multiaddr, error) {
	ipScheme := "ip4"
	if len(node.IP()) == net.IPv6len {
		ipScheme = "ip6"
	}
	pubkey := node.Pubkey()
	peerID := PeerIDFromPubkey(pubkey)
	multiAddrStr := fmt.Sprintf("/%s/%s/tcp/%d/p2p/%s", ipScheme, node.IP().String(), node.TCP(), peerID)
	multiAddr, err := ma.NewMultiaddr(multiAddrStr)
	if err != nil {
		return nil, err
	}
	return multiAddr, nil
}

// Create an ENR. All arguments are optional.
func MakeENR(ip net.IP, tcpPort uint16, udpPort uint16, priv *ecdsa.PrivateKey) *enr.Record {
	var rec enr.Record
	if ip != nil {
		if len(ip) == net.IPv4len {
			rec.Set(enr.IDv4)
		} else {
			rec.Set(enr.ID("v6"))
		}
	}
	if ip != nil {
		rec.Set(enr.IP(ip))
	}
	if priv != nil {
		pub := priv.Public().(*ecdsa.PublicKey)
		rec.Set(enode.Secp256k1(*pub))
	}
	if tcpPort != 0 {
		rec.Set(enr.TCP(tcpPort))
	}
	if udpPort != 0 {
		rec.Set(enr.UDP(udpPort))
	}
	_ = enode.SignV4(&rec, priv)
	return &rec
}
