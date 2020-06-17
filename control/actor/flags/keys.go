package flags

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/protolambda/rumor/p2p/addrutil"
)

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
