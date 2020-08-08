package custom

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/transport"
	swarm "github.com/libp2p/go-libp2p-swarm"
)

type NetworkOpts struct {
	Peerstore peerstore.Peerstore
	Reporter  metrics.Reporter
}

func NewNetwork(ctx context.Context, nodeOpts *NodeOpts, opts *NetworkOpts) (transport.TransportNetwork, error) {
	if opts.Peerstore == nil {
		return nil, fmt.Errorf("no peerstore specified")
	}

	if nodeOpts.PeerKey == nil {
		return nil, fmt.Errorf("no peer key specified")
	}

	// Obtain Peer ID from public key
	pub := nodeOpts.PeerKey.GetPublic()
	pid, err := peer.IDFromPublicKey(pub)
	if err != nil {
		return nil, err
	}

	if err := opts.Peerstore.AddPrivKey(pid, nodeOpts.PeerKey); err != nil {
		return nil, err
	}
	if err := opts.Peerstore.AddPubKey(pid, pub); err != nil {
		return nil, err
	}

	// TODO: Make the swarm implementation configurable.
	swrm := swarm.NewSwarm(ctx, pid, opts.Peerstore, opts.Reporter, nodeOpts.ConnectionGater)

	return swrm, nil
}
