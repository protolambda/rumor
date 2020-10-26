package enr

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/sirupsen/logrus"
)

type EnrViewCmd struct {
	*base.Base
	Lazy *LazyEnrState

	KvMode bool          `ask:"--kv" help:"Print the full set of Key-Value pairs"`
	Enr    flags.EnrFlag `ask:"[enr]" help:"The ENR to view, url-base64 (RFC 4648). With optional 'enr:' or 'enr://' prefix."`
}

func (c *EnrViewCmd) Help() string {
	return "View ENR contents, defaults to current actor's ENR if none provided to decode"
}

func (c *EnrViewCmd) Run(ctx context.Context, args ...string) error {
	if c.Enr.ENR == nil && c.Lazy.Current != nil {
		c.Enr.ENR = c.Lazy.Current.LocalNode().Node().Record()
	}
	if c.Enr.ENR == nil {
		return errors.New("no ENR was specified, current actor does not have one either")
	}

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
				c.Log.WithField("enr_unknown_"+key, rawValue).Infof("Unrecognized ENR KV pair type: %s", key)
			} else {
				typedValue, getValueStr := getTypedValue()
				if err := rlp.DecodeBytes(rawValue, typedValue); err != nil {
					c.Log.WithField("enr_fail_"+key, rawValue).Errorf("Failed to decode ENR KV pair: %s", key)
				}
				enrKV[key] = getValueStr()
			}
		}

		c.Log.WithField("raw_enr_kv", enrKVraw).Info("Raw ENR key-value pairs")
		c.Log.WithField("enr_kv", enrKV).Info("Decoded ENR key-value pairs")
	}

	var enodeRes *enode.Node
	enodeRes, err := addrutil.EnrToEnode(c.Enr.ENR, true)
	if err != nil {
		return err
	}

	pubkey := enodeRes.Pubkey()
	peerID := addrutil.PeerIDFromPubkey(pubkey)
	nodeID := enode.PubkeyToIDV4(pubkey)
	f := logrus.Fields{
		"seq":     enodeRes.Seq(),
		"xy":      fmt.Sprintf("%d %d", pubkey.X, pubkey.Y),
		"node_id": nodeID.String(),
		"peer_id": peerID.String(),
		"enode":   enodeRes.URLv4(),
		"enr":     enodeRes.String(), // formats as url-base64 ENR
	}
	muAddr, err := addrutil.EnodeToMultiAddr(enodeRes)
	if err != nil {
		c.Log.WithError(err).Warn("ENR has invalid multi-addr")
	} else {
		f["multi"] = muAddr.String()
	}
	c.Log.WithFields(f).Info("ENR parsed successfully")
	return nil
}
