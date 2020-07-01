package peer

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/track"
)

type PeerListCmd struct {
	*base.Base
	Store track.ExtendedPeerstore

	Which   string `ask:"[which]" help:"Which peers to list, possible values: 'all', 'connected'."`
	Details bool   `ask:"--details" help:"List detailed data of each peer"`
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
	if c.Details {
		c.Log.WithField("peers", peers).Infof("%d peers", len(peers))
	} else {
		peerData := make(map[peer.ID]*track.PeerAllData)
		for _, p := range peers {
			peerData[p] = c.Store.GetAllData(p)
		}
		c.Log.WithField("peers", peerData).Infof("%d peers", len(peers))
	}
	return nil
}
