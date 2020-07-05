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
	return "List peers."
}

func (c *PeerListCmd) Default() {
	c.Which = "connected"
}

func (c *PeerListCmd) Run(ctx context.Context, args ...string) error {
	h, hostErr := c.Host()
	var peers []peer.ID
	switch c.Which {
	case "all":
		peers = c.Store.Peers()
	case "connected":
		if hostErr != nil {
			return hostErr
		}
		peers = h.Network().Peers()
	default:
		return fmt.Errorf("invalid peer selection type: %s", c.Which)
	}
	if c.Details {
		peerData := make(map[string]*track.PeerAllData)
		for _, p := range peers {
			peerData[p.String()] = c.Store.GetAllData(p)
		}
		c.Log.WithField("peers", peerData).Infof("%d peers", len(peers))
	} else {
		c.Log.WithField("peers", peers).Infof("%d peers", len(peers))
	}
	return nil
}
