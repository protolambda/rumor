package kad

import (
	"context"
	"eth2-lurk/node"
	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	kad_dht "github.com/libp2p/go-libp2p-kad-dht"
	dhtopts "github.com/libp2p/go-libp2p-kad-dht/opts"
	"github.com/sirupsen/logrus"
)

type Kademlia interface {
	ProtocolID() protocol.ID
	RefreshTable(wait bool)
	FindPeer(ctx context.Context, id peer.ID) (peer.AddrInfo, error)
	FindPeersConnectedToPeer(ctx context.Context, id peer.ID) (<-chan *peer.AddrInfo, error)
	// TODO more methods to connect to nodes etc.
}

type KademliaImpl struct {
	protocolID protocol.ID
	dhtData *kad_dht.IpfsDHT
	log logrus.FieldLogger
}

func NewKademlia(ctx context.Context, n node.Node, id protocol.ID) (Kademlia, error) {
	// example protocol id: "/prysm/0.0.0/dht"
	dhtOpts := []dhtopts.Option{
		dhtopts.Datastore(ds_sync.MutexWrap(ds.NewMapDatastore())), // instead of the default map datastore.
		dhtopts.Protocols(id), // don't creep onto the default IPFS network, join the configured DHT
	}
	logger := n.Logger("kademlia")
	kd, err := kad_dht.New(ctx, n.Host(), dhtOpts...)
	if err != nil {
		logger.Errorf("Failed to start Kademlia DHT, protocol: %s", id)
		return nil, err
	}
	logger.Infof("started Kademlia DHT, protocol: %s", id)
	return &KademliaImpl{
		protocolID: id,
		dhtData: kd,
		log:     logger,
	}, nil
}

func (kad *KademliaImpl) ProtocolID() protocol.ID {
	return kad.protocolID
}

func (kad *KademliaImpl) FindPeer(ctx context.Context, id peer.ID) (peer.AddrInfo, error) {
	return kad.dhtData.FindPeer(ctx, id)
}

func (kad *KademliaImpl) FindPeersConnectedToPeer(ctx context.Context, id peer.ID) (<-chan *peer.AddrInfo, error) {
	return kad.dhtData.FindPeersConnectedToPeer(ctx, id)
}

func (kad *KademliaImpl) RefreshTable(wait bool) {
	refResult := kad.dhtData.RefreshRoutingTable()

	// Result is safe to ignore but interesting to log.
	waitForResult := func() {
		err := <-refResult
		if err != nil {
			kad.log.Errorf("failed to refresh kad dht table: %v", err)
		} else {
			kad.log.Info("successfully refreshed kad dht table")
		}
	}
	if wait {
		waitForResult()
	} else {
		go waitForResult()
	}
}
