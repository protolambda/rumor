package peerstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/sync"
	badger "github.com/ipfs/go-ds-badger"
	leveldb "github.com/ipfs/go-ds-leveldb"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-peerstore/pstoreds"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/protolambda/rumor/p2p/track/dstrack"
	"strings"
)

type CreateCmd struct {
	*base.Base

	GlobalPeerstores track.Peerstores
	CurrentPeerstore track.DynamicPeerstore

	ID        track.PeerstoreID `ask:"[id]" help:"ID of the peerstore, random otherwise"`
	Switch    bool              `ask:"--switch" help:"If the host should immediately switch to the newly created peerstore"`
	StoreType string            `ask:"--store-type" help:"The type of datastore to use. Options: 'mem', 'leveldb', 'badger'"`
	StorePath string            `ask:"--store-path" help:"The path of the datastore, must be empty for memory store."`
}

func (c *CreateCmd) Default() {
	c.Switch = true
	c.StoreType = "mem"
}

func (c *CreateCmd) Help() string {
	return "Create and activate a peerstore"
}

func (c *CreateCmd) Run(ctx context.Context, args ...string) error {
	c.StorePath = strings.TrimSpace(c.StorePath)
	if (c.StoreType == "leveldb" || c.StoreType == "badger") && c.StorePath == "" {
		return fmt.Errorf("store type '%s' requires a store path to be set", c.StoreType)
	}
	var store ds.Batching
	switch c.StoreType {
	case "", "mem":
		store = sync.MutexWrap(ds.NewMapDatastore())
		if c.StorePath != "" {
			return errors.New("memory peerstore cannot have store path")
		}
	case "leveldb":
		var err error
		store, err = leveldb.NewDatastore(c.StorePath, nil)
		if err != nil {
			return err
		}
	case "badger":
		var err error
		store, err = badger.NewDatastore(c.StorePath, nil)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unrecognized store type: %s", c.StoreType)
	}
	ep, err := dstrack.NewExtendedPeerstore(c.GlobalContext, store, pstoreds.DefaultOpts())
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
	if c.Switch {
		c.CurrentPeerstore.Switch(retainId, id, ep)
	}
	return nil
}
