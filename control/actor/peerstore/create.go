package peerstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-peerstore/pstoreds"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/protolambda/rumor/p2p/track/dstrack"
)

type CreateCmd struct {
	*base.Base

	GlobalPeerstores *track.Peerstores
	CurrentPeerstore track.DynamicPeerstore

	ID track.PeerstoreID `ask:"--id" help:"ID of the peerstore, random otherwise"`
}

func (c *CreateCmd) Help() string {
	return "Create and activate a peerstore"
}

func (c *CreateCmd) Run(ctx context.Context, args ...string) error {
	st := ds.NewMapDatastore()
	ep, err := dstrack.NewExtendedPeerstore(c.GlobalContext, st, pstoreds.DefaultOpts())
	if err != nil {
		return fmt.Errorf("failed to build datastore-backed peerstore named: %v", err)
	}
	id := c.ID
	if id == "" {
		var dat [24]byte
		if _, err := rand.Read(dat[:]); err != nil {
			return fmt.Errorf("failed to get random peerstore ID: %v", err)
		}
		id = track.PeerstoreID(hex.EncodeToString(dat[:]))
	}
	if err := c.GlobalPeerstores.Create(id, ep); err != nil {
		return fmt.Errorf("failed to share peerstore: %v", err)
	}
	h, err := c.Host()
	var retainId peer.ID
	if err == nil {
		retainId = h.ID()
	}
	c.CurrentPeerstore.Switch(retainId, id, ep)
	return nil
}
