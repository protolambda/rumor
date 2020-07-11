package dv5

import (
	"encoding/hex"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/sirupsen/logrus"
	"time"
)

type HandleENR struct {
	Store track.ExtendedPeerstore

	Add          bool              `ask:"--add" help:"Add the discovered nodes to the peerstore"`
	FilterDigest beacon.ForkDigest `ask:"--filter-digest" help:"Only add peers with the given digest to the peerstore"`
	TTL          time.Duration     `ask:"--ttl" help:"When adding the node, apply this TTL"`
	Filtering    bool              `changed:"filter-digest"`
}

func (c *HandleENR) handle(log logrus.FieldLogger, h host.Host, res *enode.Node) {
	pubkey := res.Pubkey()
	peerID := addrutil.PeerIDFromPubkey(pubkey)
	if c.Add {
		eth2Dat, ok, err := addrutil.ParseEnrEth2Data(res)
		if err != nil {
			log.WithFields(logrus.Fields{"enr": res.String(), "id": res.ID().String()}).Warnf("enr parse error: %v", err)
			return
		}
		if !ok && c.Filtering {
			log.WithFields(logrus.Fields{"enr": res.String(), "id": res.ID().String()}).Warn("got ENR without fork digest")
			return
		}
		if c.Filtering {
			if eth2Dat.ForkDigest != c.FilterDigest {
				log.WithFields(logrus.Fields{"enr": res.String(), "id": res.ID().String(),
					"digest": hex.EncodeToString(eth2Dat.ForkDigest[:])}).Warn("got ENR with other fork digest")
				return
			}
		}
		updated, err := c.Store.UpdateENRMaybe(peerID, res)
		if err != nil {
			log.WithFields(logrus.Fields{"enr": res.String(), "id": res.ID().String()}).Warnf("enr update error: %v", err)
			return
		}
		if updated {
			addr, err := addrutil.EnodeToMultiAddr(res)
			if err != nil {
				log.WithFields(logrus.Fields{"enr": res.String(), "id": res.ID().String()}).Warnf("failed to parse ENR address into multi-addr for libp2p: %v", err)
				return
			}
			h.Peerstore().SetAddr(peerID, addr, c.TTL)
			log.WithFields(logrus.Fields{"id": res.ID().String()}).Infof("Updated ENR record")
		}
	}
}
