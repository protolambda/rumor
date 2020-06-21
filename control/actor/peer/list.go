package peer

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/rumor/control/actor/base"
)

type PeerListCmd struct {
	*base.Base
	WithPeerInfos

	Which string `ask:"[which]" help:"Which peers to list, possible values: 'all', 'connected'."`

	ListLatency   bool `ask:"--latency" help:"list peer latency"`
	ListProtocols bool `ask:"--protocols" help:"list peer protocols"`
	ListAddrs     bool `ask:"--addrs" help:"list peer addrs"`
	ListStatus    bool `ask:"--status" help:"list peer status"`
	ListMetadata  bool `ask:"--metadata" help:"list peer metadata"`
	ListClaimSeq  bool `ask:"--claimseq" help:"list peer claimed metadata seq nr"`
}

func (c *PeerListCmd) Help() string {
	return "Stop the host node."
}

func (c *PeerListCmd) Default() {
	c.Which = "connected"
}

func (c *PeerListCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		args = append(args, "connected")
	}
	var peers []peer.ID
	switch args[0] {
	case "all":
		peers = h.Peerstore().Peers()
	case "connected":
		peers = h.Network().Peers()
	default:
		return fmt.Errorf("invalid peer type: %s", args[0])
	}
	store := h.Peerstore()
	peerData := make(map[peer.ID]map[string]interface{})
	for _, p := range peers {
		v := make(map[string]interface{})
		if c.ListAddrs {
			v["addrs"] = store.PeerInfo(p).Addrs
		}
		// TODO: add dv5 node ID
		if c.ListLatency {
			v["latency"] = store.LatencyEWMA(p).Seconds() // A float, ok for json
		}
		if c.ListProtocols {
			protocols, err := store.GetProtocols(p)
			if err != nil {
				v["protocols"] = protocols
			}
		}
		if c.ListStatus || c.ListMetadata || c.ListClaimSeq {
			pInfoData, ok := c.WithPeerInfos.Find(p)
			if ok {
				if c.ListStatus {
					v["status"] = pInfoData.Status()
				}
				if c.ListMetadata {
					v["metadata"] = pInfoData.Metadata()
				}
				if c.ListClaimSeq {
					v["metadata"] = pInfoData.ClaimedSeq()
				}
			}
		}
		peerData[p] = v
	}
	c.Log.WithField("peers", peerData).Infof("%d peers", len(peers))
	return nil
}
