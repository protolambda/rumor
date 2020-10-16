package enr

import (
	"context"
	"crypto/ecdsa"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	gcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/protolambda/rumor/control/actor/base"
)

type EnrGenKeyCmd struct {
	*base.Base
}

func (c *EnrGenKeyCmd) Help() string {
	return "Make a private key for an ENR."
}

func (c *EnrGenKeyCmd) Run(ctx context.Context, args ...string) error {
	key, err := ecdsa.GenerateKey(gcrypto.S256(), crand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate key: %v", err)
	}
	secpKey := (*crypto.Secp256k1PrivateKey)(key)
	keyBytes, err := secpKey.Raw()
	if err != nil {
		return fmt.Errorf("failed to serialize key: %v", err)
	}
	c.Log.WithField("key", hex.EncodeToString(keyBytes)).Infoln("generated key")
	return nil
}
