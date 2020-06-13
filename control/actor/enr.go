package actor

import (
	"context"
	"crypto/ecdsa"
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/btcec"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/sirupsen/logrus"
	"net"
)

type EnrCmd struct {
	*Actor `ask:"-"`
	log    logrus.FieldLogger
}

func (c *EnrCmd) Get(ctx context.Context, args ...string) (cmd interface{}, remaining []string, err error) {
	if len(args) == 0 {
		return nil, nil, errors.New("no subcommand specified")
	}
	switch args[0] {
	case "view":
		cmd = &EnrViewCmd{EnrCmd: c}
	case "gen-key":
		cmd = &EnrGenKeyCmd{EnrCmd: c}
	case "make":
		cmd = &EnrMakeCmd{EnrCmd: c}
	default:
		return nil, args, fmt.Errorf("unrecognized command: %v", args)
	}
	return cmd, args[1:], nil
}

func (c *EnrCmd) Help() string {
	return "Ethereum Name Record (ENR) utilities" // TODO list subcommands
}


type EnrViewCmd struct {
	*EnrCmd `ask:"-"`
	KvMode bool `ask:"--kv" help:"Print the full set of Key-Value pairs"`
	Enr EnrFlag `ask:"<enr>" help:"The ENR to view, url-base64 (RFC 4648). With optional 'enr:' or 'enr://' prefix."`
}

func (c *EnrViewCmd) Help() string {
	return "View ENR contents"
}

func (c *EnrViewCmd) Run(ctx context.Context, args ...string) error {
	if c.KvMode {
		enrPairs := c.Enr.ENR.AppendElements(nil)
		enrKV := make(map[string]string)
		enrKVraw := make(map[string]string)
		for i := 1; i < len(enrPairs); i += 2 {
			key := enrPairs[i].(string)
			rawValue := enrPairs[i+1].(rlp.RawValue)

			enrKVraw[key] = hex.EncodeToString(rawValue)
			getTypedValue, ok := addrutil.EnrEntries[key]
			if !ok {
				c.log.WithField("enr_unknown_"+key, rawValue).Infof("Unrecognized ENR KV pair type: %s", key)
			} else {
				typedValue, getValueStr := getTypedValue()
				if err := rlp.DecodeBytes(rawValue, typedValue); err != nil {
					c.log.WithField("enr_fail_"+key, rawValue).Errorf("Failed to decode ENR KV pair: %s", key)
				}
				enrKV[key] = getValueStr()
			}
		}

		c.log.WithField("raw_enr_kv", enrKVraw).Info("Raw ENR key-value pairs")
		c.log.WithField("enr_kv", enrKV).Info("Decoded ENR key-value pairs")
	}

	var enodeRes *enode.Node
	enodeRes, err := addrutil.EnrToEnode(c.Enr.ENR, true)
	if err != nil {
		return err
	}

	pubkey := enodeRes.Pubkey()
	peerID := addrutil.PeerIDFromPubkey(pubkey)
	nodeID := enode.PubkeyToIDV4(pubkey)
	muAddr, err := addrutil.EnodeToMultiAddr(enodeRes)
	if err != nil {
		return err
	}
	c.log.WithFields(logrus.Fields{
		"seq":     enodeRes.Seq(),
		"xy":      fmt.Sprintf("%d %d", pubkey.X, pubkey.Y),
		"node_id": nodeID.String(),
		"peer_id": peerID.String(),
		"enode":   enodeRes.URLv4(),
		"multi":   muAddr.String(),
		"enr":     enodeRes.String(), // formats as url-base64 ENR
	}).Info("ENR parsed successfully")
	return nil
}


type EnrGenKeyCmd struct {
	*EnrCmd `ask:"-"`
}

func (c *EnrGenKeyCmd) Help() string {
	return "Make a private key for an ENR."
}

func (c *EnrGenKeyCmd) Run(ctx context.Context, args ...string) error {
	key, err := ecdsa.GenerateKey(btcec.S256(), crand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate key: %v", err)
	}
	secpKey := (*crypto.Secp256k1PrivateKey)(key)
	keyBytes, err := secpKey.Raw()
	if err != nil {
		return fmt.Errorf("failed to serialize key: %v", err)
	}
	c.log.WithField("key", hex.EncodeToString(keyBytes)).Infoln("generated key")
	return nil
}

type EnrMakeCmd struct {
	*EnrCmd `ask:"-"`
	IP net.IP `ask:"--ip" help:"IP address (v4 or v6). None if empty."`
	TCP uint16 `ask:"--tcp" help:"TCP port. None if 0"`
	UDP uint16 `ask:"--udp" help:"UDP port. None if 0"`
	Priv P2pPrivKeyFlag `ask:"--priv" help:"Private key, in raw hex encoded format (32 bytes -> 64 hex chars with optional 0x prefix). None if empty, and also no signed ENR."`
}

func (c *EnrMakeCmd) Help() string {
	return "Make a private key for an ENR."
}

func (c *EnrMakeCmd) Run(ctx context.Context, args ...string) error {
	rec := addrutil.MakeENR(c.IP, c.TCP, c.UdpPort, c.Priv.Priv)
	enrStr, err := addrutil.EnrToString(rec)
	if err != nil {
		return err
	}
	c.log.WithField("enr", enrStr).Infof("ENR created!")
	return nil
}

