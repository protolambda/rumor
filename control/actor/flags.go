package actor

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"strings"
)

type EnrFlag struct {
	ENR *enr.Record
}

func (f *EnrFlag) String() string {
	if f == nil {
		return "nil ENR"
	}
	enrStr, err := addrutil.EnrToString(f.ENR)
	if err != nil {
		return "? (invalid ENR)"
	}
	return enrStr
}

func (f *EnrFlag) Set(v string) error {
	enrAddr, err := addrutil.ParseEnr(v)
	if err != nil {
		return err
	}
	f.ENR = enrAddr
	return nil
}

func (f *EnrFlag) Type() string {
	return "ENR"
}

type EnodeFlag struct {
	Enode *enode.Node
}

func (f *EnodeFlag) String() string {
	if f == nil {
		return "nil enode"
	}
	return f.Enode.String()
}

func (f *EnodeFlag) Set(v string) error {
	en, err := addrutil.ParseEnode(v)
	if err != nil {
		return err
	}
	f.Enode = en
	return nil
}

func (f *EnodeFlag) Type() string {
	return "enode"
}

type EnrOrEnodeFlag struct {
	Enode *enode.Node
}

func (f *EnrOrEnodeFlag) String() string {
	if f == nil {
		return "nil enode"
	}
	return f.Enode.String()
}

func (f *EnrOrEnodeFlag) Set(v string) error {
	en, err := addrutil.ParseEnrOrEnode(v)
	if err != nil {
		return err
	}
	f.Enode = en
	return nil
}

func (f *EnrOrEnodeFlag) Type() string {
	return "Enr/Enode"
}

type NodeIDFlag struct {
	ID enode.ID
}

func (f *NodeIDFlag) String() string {
	if f == nil {
		return "nil node ID"
	}
	return f.ID.String()
}

func (f *NodeIDFlag) Set(v string) error {
	id, err := addrutil.ParseNodeID(v)
	if err != nil {
		return err
	}
	f.ID = id
	return nil
}

func (f *NodeIDFlag) Type() string {
	return "Dv5 node ID"
}

type NodeIDFlexibleFlag struct {
	ID enode.ID
}

func (f *NodeIDFlexibleFlag) String() string {
	if f == nil {
		return "nil node ID"
	}
	return f.ID.String()
}

func (f *NodeIDFlexibleFlag) Set(v string) error {
	id, err := addrutil.ParseNodeIDOrEnrOrEnode(v)
	if err != nil {
		return err
	}
	f.ID = id
	return nil
}

func (f *NodeIDFlexibleFlag) Type() string {
	return "Dv5 flexible ID"
}


type PeerIDFlag struct {
	PeerID peer.ID
}

func (f *PeerIDFlag) String() string {
	if f == nil {
		return "nil peer id"
	}
	return f.PeerID.String()
}

func (f *PeerIDFlag) Set(v string) error {
	id, err := peer.Decode(v)
	if err != nil {
		return err
	}
	f.PeerID = id
	return nil
}

func (f *PeerIDFlag) Type() string {
	return "Peer ID"
}



type CompressionFlag struct {
	Compression reqresp.Compression
}

func (f *CompressionFlag) String() string {
	if f == nil {
		return "nil compression"
	}
	if f.Compression == nil {
		return "none"
	}
	return f.Compression.Name()
}

func (f *CompressionFlag) Set(v string) error {
	if v == "snappy" {
		f.Compression = reqresp.SnappyCompression{}
		return nil
	} else if v == "none" || v == "" {
		f.Compression = nil
		return nil
	} else {
		return fmt.Errorf("unrecognized compression: %s", v)
	}
}

func (f *CompressionFlag) Type() string {
	return "RPC compression"
}

// BytesHex exposes bytes as a flag, hex-encoded,
// optional whitespace padding, case insensitive, and optional 0x prefix.
type BytesHexFlag []byte

func (f BytesHexFlag) String() string {
	return hex.EncodeToString(f)
}

func (f *BytesHexFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	value = strings.ToLower(value)
	if strings.HasPrefix(value, "0x") {
		value = value[2:]
	}
	b, err := hex.DecodeString(value)
	if err != nil {
		return err
	}
	*f = b
	return nil
}

type P2pPrivKeyFlag struct {
	Priv *ecdsa.PrivateKey
}

func (f P2pPrivKeyFlag) String() string {
	if f.Priv == nil {
		return "? (no private key)"
	}
	secpKey := (*crypto.Secp256k1PrivateKey)(f.Priv)
	keyBytes, err := secpKey.Raw()
	if err != nil {
		return "? (invalid private key)"
	}
	return hex.EncodeToString(keyBytes)
}

func (f *P2pPrivKeyFlag) Set(value string) error {
	var priv *ecdsa.PrivateKey
	var err error
	priv, err = addrutil.ParsePrivateKey(value)
	if err != nil {
		return fmt.Errorf("could not parse private key: %v", err)
	}
	f.Priv = priv
	return nil
}

func (f *P2pPrivKeyFlag) Type() string {
	return "P2P Private key"
}
