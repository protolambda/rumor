package peer

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/sirupsen/logrus"
)

type PeerInfoCmd struct {
	*base.Base
	Store track.ExtendedPeerstore

	PeerID flags.PeerIDFlag `ask:"<peer-ID>" help:"peer ID"`
}

func (c *PeerInfoCmd) Help() string {
	return "Read info about a specific peer from the peerstore."
}

func (c *PeerInfoCmd) Run(ctx context.Context, args ...string) error {
	info := c.Store.GetAllData(c.PeerID.PeerID)
	f := logrus.Fields{}
	f["peer_id"] = info.PeerID
	f["node_id"] = info.NodeID
	f["pubkey"] = info.Pubkey
	if info.Addrs != nil {
		f["addrs"] = info.Addrs
	}
	if info.Protocols != nil {
		f["protocols"] = info.Protocols
	}
	if info.Latency != 0 {
		f["latency"] = info.Latency
	}
	if info.UserAgent != "" {
		f["user_agent"] = info.UserAgent
	}
	if info.ProtocolVersion != "" {
		f["protocol_version"] = info.ProtocolVersion
	}

	if info.ForkDigest != nil {
		f["enr_fork_digest"] = info.ForkDigest
	}
	if info.NextForkVersion != nil {
		f["enr_next_fork_version"] = info.NextForkVersion
	}
	if info.NextForkEpoch != nil {
		f["enr_next_fork_epoch"] = info.NextForkEpoch
	}

	if info.Attnets != nil {
		f["enr_attnets"] = info.Attnets
	}
	if info.MetaData != nil {
		f["metadata"] = info.MetaData
	}
	if info.ClaimedSeq != 0 {
		f["claimed_seq"] = info.ClaimedSeq
	}
	if info.Status != nil {
		f["status"] = info.Status
	}
	if info.ENR != nil {
		f["enr"] = info.ENR
	}
	c.Log.WithFields(f).Infof("peer info")
	return nil
}
