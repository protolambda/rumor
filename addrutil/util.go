package addrutil

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/btcec"
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

func ParseEnrOrEnode(v string) (*enode.Node, error) {
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
