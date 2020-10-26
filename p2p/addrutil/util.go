package addrutil

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/btcec"
	gcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/ztyp/codec"
	"net"
	"strings"
)

type Eth2ENREntry []byte

func NewEth2DataEntry(dat *beacon.Eth2Data) Eth2ENREntry {
	var buf bytes.Buffer
	if err := dat.Serialize(codec.NewEncodingWriter(&buf)); err != nil {
		return nil
	}
	return buf.Bytes()
}

func (eee Eth2ENREntry) ENRKey() string {
	return "eth2"
}

func (eee Eth2ENREntry) Eth2Data() (*beacon.Eth2Data, error) {
	var dat beacon.Eth2Data
	if err := dat.Deserialize(codec.NewDecodingReader(bytes.NewReader(eee), uint64(len(eee)))); err != nil {
		return nil, err
	}
	return &dat, nil
}

func (eee Eth2ENREntry) String() string {
	dat, err := eee.Eth2Data()
	if err != nil {
		return fmt.Sprintf("invalid eth2 data! Raw: %x", eee[:])
	}
	return fmt.Sprintf("digest: %s, next fork version: %s, next fork epoch: %d",
		dat.ForkDigest, dat.NextForkVersion, dat.NextForkEpoch)
}

type AttnetsENREntry []byte

func NewAttnetsENREntry(dat *beacon.AttnetBits) AttnetsENREntry {
	var buf bytes.Buffer
	if err := dat.Serialize(codec.NewEncodingWriter(&buf)); err != nil {
		return nil
	}
	return buf.Bytes()
}

func (aee AttnetsENREntry) ENRKey() string {
	return "attnets"
}

func (aee AttnetsENREntry) AttnetBits() (beacon.AttnetBits, error) {
	var dat beacon.AttnetBits
	if err := dat.Deserialize(codec.NewDecodingReader(bytes.NewReader(aee), uint64(len(aee)))); err != nil {
		return beacon.AttnetBits{}, err
	}
	return dat, nil
}

func (aee AttnetsENREntry) String() string {
	return hex.EncodeToString(aee)
}

var EnrEntries = map[string]func() (enr.Entry, func() string){
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
	"attnets": func() (enr.Entry, func() string) {
		res := new(AttnetsENREntry)
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

func ParseEnode(v string) (*enode.Node, error) {
	addr := new(enode.Node)
	err := addr.UnmarshalText([]byte(v))
	if err != nil {
		return nil, err
	}
	return addr, nil
}

func ParseEnrOrEnode(v string) (*enode.Node, error) {
	if strings.HasPrefix(v, "enode://") {
		return ParseEnode(v)
	} else {
		enrAddr, err := ParseEnr(v)
		if err != nil {
			return nil, err
		}
		enodeAddr, err := EnrToEnode(enrAddr, true)
		if err != nil {
			return nil, err
		}
		return enodeAddr, nil
	}
}

func ParseNodeID(v string) (enode.ID, error) {
	if h, err := hex.DecodeString(v); err != nil {
		return enode.ID{}, fmt.Errorf("provided target node is not a valid node ID: %s", v)
	} else {
		if len(h) != 32 {
			return enode.ID{}, fmt.Errorf("hex node ID is not 32 bytes: %s", v)
		} else {
			var out enode.ID
			copy(out[:], h)
			return out, nil
		}
	}
}

func ParseNodeIDOrEnrOrEnode(v string) (enode.ID, error) {
	if h, err := hex.DecodeString(v); err != nil {
		en, err := ParseEnrOrEnode(v)
		if err != nil {
			return enode.ID{}, fmt.Errorf("provided target node is not a valid node ID, enode address or ENR: %s", v)
		}
		return en.ID(), nil
	} else {
		if len(h) != 32 {
			return enode.ID{}, fmt.Errorf("hex node ID is not 32 bytes: %s", v)
		} else {
			var out enode.ID
			copy(out[:], h)
			return out, nil
		}
	}
}

func ParsePrivateKey(v string) (*crypto.Secp256k1PrivateKey, error) {
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
	key := (priv).(*crypto.Secp256k1PrivateKey)
	key.Curve = gcrypto.S256()              // Temporary hack, so libp2p Secp256k1 is recognized as geth Secp256k1 in disc v5.1
	if !key.Curve.IsOnCurve(key.X, key.Y) { // TODO: should we be checking this?
		return nil, fmt.Errorf("invalid private key, not on curve")
	}
	return key, nil
}

func PrivKeysEqual(a *crypto.Secp256k1PrivateKey, b *crypto.Secp256k1PrivateKey) (ok bool, err error) {
	if a == nil {
		if b == nil {
			return true, nil
		} else {
			return false, nil
		}
	}
	aRaw, err := a.Raw()
	if err != nil {
		return false, err
	}
	bRaw, err := b.Raw()
	if err != nil {
		return false, err
	}
	return bytes.Equal(aRaw, bRaw), nil
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
	// TODO: struct init, instead of type cast
	id, _ := peer.IDFromPublicKey(crypto.PubKey((*crypto.Secp256k1PublicKey)((*btcec.PublicKey)(pubkey))))
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
func MakeENR(ip net.IP, tcpPort uint16, udpPort uint16, priv *crypto.Secp256k1PrivateKey) *enr.Record {
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
		pub := (*ecdsa.PrivateKey)(priv).Public().(*ecdsa.PublicKey)
		rec.Set(enode.Secp256k1(*pub))
	}
	if tcpPort != 0 {
		rec.Set(enr.TCP(tcpPort))
	}
	if udpPort != 0 {
		rec.Set(enr.UDP(udpPort))
	}
	if priv != nil {
		_ = enode.SignV4(&rec, (*ecdsa.PrivateKey)(priv))
	}
	return &rec
}

func ParseEnrEth2Data(n *enode.Node) (data *beacon.Eth2Data, exists bool, err error) {
	var eth2 Eth2ENREntry
	if err := n.Load(&eth2); err != nil {
		return nil, false, nil
	}
	dat, err := eth2.Eth2Data()
	if err != nil {
		return nil, true, fmt.Errorf("failed parsing eth2 bytes: %v", err)
	}
	return dat, true, nil
}

func ParseEnrAttnets(n *enode.Node) (attnetbits *beacon.AttnetBits, exists bool, err error) {
	var attnets AttnetsENREntry
	if err := n.Load(&attnets); err != nil {
		return nil, false, nil
	}
	dat, err := attnets.AttnetBits()
	if err != nil {
		return nil, true, fmt.Errorf("failed parsing attnets bytes: %v", err)
	}
	return &dat, true, nil
}
