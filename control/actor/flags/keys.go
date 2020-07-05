package flags

import (
	"encoding/hex"
	"fmt"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/protolambda/rumor/p2p/addrutil"
)

type P2pPrivKeyFlag struct {
	Priv *crypto.Secp256k1PrivateKey
}

func (f P2pPrivKeyFlag) String() string {
	if f.Priv == nil {
		return "? (no private key data)"
	}
	secpKey := f.Priv
	keyBytes, err := secpKey.Raw()
	if err != nil {
		return "? (invalid private key)"
	}
	return hex.EncodeToString(keyBytes)
}

func (f *P2pPrivKeyFlag) Set(value string) error {
	// No private key if no data
	if value == "" {
		f.Priv = nil
		return nil
	}
	var priv *crypto.Secp256k1PrivateKey
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
