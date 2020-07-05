package peer

import (
	"context"
	"errors"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/track"
	"time"
)

type PeerAddCmd struct {
	*base.Base
	Store  track.ExtendedPeerstore
	PeerID flags.PeerIDFlag       `ask:"--peer-id" help:"If not present in the address, manually specify the peer ID."`
	TTL    time.Duration          `ask:"--ttl" help:"How long the address should be kept around."`
	Addr   flags.FlexibleAddrFlag `ask:"<addr>" help:"ENR, enode or multi address to add."`
}

func (c *PeerAddCmd) Help() string {
	return "Add the address of a peer to the peerstore. If an ENR is provided, update ENR values as well."
}

func (c *PeerAddCmd) Default() {
	c.TTL = time.Hour * 24 * 20
}

func (c *PeerAddCmd) Run(ctx context.Context, args ...string) error {
	addr, id := peer.SplitAddr(c.Addr.MultiAddr)
	if id == "" {
		id = c.PeerID.PeerID
	}
	if id == "" {
		return errors.New("missing peer ID")
	}
	c.Store.AddAddr(id, addr, c.TTL)
	if c.Addr.OptionalEnr != nil {
		if updated, err := c.Store.UpdateENRMaybe(id, c.Addr.OptionalEnr); err != nil {
			return fmt.Errorf("failed to update ENR data: %v", err)
		} else if updated {
			c.Log.Info("updated ENR data in peerstore")
		}
	}
	return nil
}
