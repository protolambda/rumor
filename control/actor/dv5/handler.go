package dv5

import (
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/sirupsen/logrus"
	"time"
)

type HandleENR struct {
	Store track.DynamicPeerstore

	Add          bool              `ask:"--add" help:"Add the discovered nodes to the peerstore (requires peerstore to use)"`
	FilterDigest beacon.ForkDigest `ask:"--filter-digest" help:"Only add peers with the given digest to the peerstore"`
	TTL          time.Duration     `ask:"--ttl" help:"When adding the node, apply this TTL"`
	Filtering    bool              `changed:"filter-digest"`
}

func (c *HandleENR) handle(log logrus.FieldLogger, res *enode.Node) error {
	pubkey := res.Pubkey()
	peerID := addrutil.PeerIDFromPubkey(pubkey)
	if c.Add {
		if c.Store == nil || !c.Store.Initialized() {
			return errors.New("to add nodes, a peerstore is required")
		}

		eth2Dat, ok, err := addrutil.ParseEnrEth2Data(res)
		if err != nil {
			return fmt.Errorf("enr parse error: %v", err)
		}
		if !ok && c.Filtering {
			return fmt.Errorf("got ENR without fork digest")
		}
		if c.Filtering {
			if eth2Dat.ForkDigest != c.FilterDigest {
				return fmt.Errorf("got ENR with other fork digest: %s", eth2Dat.ForkDigest.String())
			}
		}
		updated, err := c.Store.UpdateENRMaybe(peerID, res)
		if err != nil {
			return fmt.Errorf("enr update error: %v", err)
		}
		if updated {
			addr, err := addrutil.EnodeToMultiAddr(res)
			if err != nil {
				return fmt.Errorf("failed to parse ENR address into multi-addr for libp2p: %v", err)
			}
			c.Store.SetAddr(peerID, addr, c.TTL)
			log.WithFields(logrus.Fields{"id": res.ID().String()}).Infof("Updated ENR record")
		}
	}
	return nil
}
