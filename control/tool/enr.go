package tool

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/addrutil"
	"io"
	"strconv"
)

type EnrValueGetCmd struct {
	Raw bool          `ask:"--raw" help:"To get the raw RLP value, instead of deserialized data"`
	Key string        `ask:"<key>" help:"The key of the value to extract from the ENR"`
	Enr flags.EnrFlag `ask:"<enr>" help:"The ENR to view, url-base64 (RFC 4648). With optional 'enr:' or 'enr://' prefix."`
	Out io.Writer
}

func (c *EnrValueGetCmd) Help() string {
	return `Get ENR value by key.
			Any key can be displayed as raw RLP. Unknown keys do not have a deserialized form, but can be extracted as RLP.

			ENR keys: secp256k1, tcp, tcp6, udp, udp6, id, ip, ip6, eth2, attnets
			Special keys: seq, signature, node_id, pubkey, enode, multi, peer_id
` // indent is intentional, displays better in help text.
}

func (c *EnrValueGetCmd) Run(ctx context.Context, args ...string) error {
	result := func(v string) error {
		_, err := c.Out.Write([]byte(v))
		return err
	}
	getTypedValue, ok := addrutil.EnrEntries[c.Key]
	if ok {
		var key string
		var rawValue rlp.RawValue
		enrPairs := c.Enr.ENR.AppendElements(nil)
		for i := 1; i < len(enrPairs); i += 2 {
			key = enrPairs[i].(string)
			if key != c.Key {
				continue
			}
			rawValue = enrPairs[i+1].(rlp.RawValue)
		}
		if c.Raw {
			return result(hex.EncodeToString(rawValue))
		} else {
			typedValue, getValueStr := getTypedValue()
			if err := rlp.DecodeBytes(rawValue, typedValue); err != nil {
				return fmt.Errorf("failed to decode ENR KV pair: '%s', err: %v", key, err)
			}
			return result(getValueStr())
		}
	}
	if c.Key == "signature" {
		return result(hex.EncodeToString(c.Enr.ENR.Signature()))
	}
	if c.Key == "seq" {
		return result(strconv.FormatUint(c.Enr.ENR.Seq(), 10))
	}

	var enodeRes *enode.Node
	enodeRes, err := addrutil.EnrToEnode(c.Enr.ENR, true)
	if err != nil {
		return err
	}
	switch c.Key {
	case "pubkey":
		b, err := (*crypto.Secp256k1PublicKey)(enodeRes.Pubkey()).Raw()
		if err != nil {
			return fmt.Errorf("pubkey reading error: %v", err)
		}
		return result(hex.EncodeToString(b))
	case "node_id":
		return result(enode.PubkeyToIDV4(enodeRes.Pubkey()).String())
	case "peer_id":
		return result(addrutil.PeerIDFromPubkey(enodeRes.Pubkey()).String())
	case "enode":
		return result(enodeRes.URLv4())
	case "multi":
		muAddr, err := addrutil.EnodeToMultiAddr(enodeRes)
		if err != nil {
			return fmt.Errorf("could not convert to multi-addr: %v", err)
		}
		return result(muAddr.String())
	default:
		return fmt.Errorf("unrecognized key: '%s'", c.Key)
	}
}
