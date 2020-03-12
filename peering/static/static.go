package static

import (
	"context"
	"github.com/protolambda/rumor/node"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
)

type ConnectionCallback func(info peer.AddrInfo, alreadyConnected bool) error

// onConnect may be nil, if no further action is required after starting the connection.
func ConnectStaticPeers(ctx context.Context, log logrus.FieldLogger, n node.Node, multiAddrs []ma.Multiaddr, onConnect ConnectionCallback) error {
	infos, err := peer.AddrInfosFromP2pAddrs(multiAddrs...)
	if err != nil {
		log.Errorf("failed to read multi addrs: %v", multiAddrs)
		return err
	}
	host := n.Host()
	for _, info := range infos {
		alreadyConnected := len(host.Network().ConnsToPeer(info.ID)) > 0
		if !alreadyConnected {
			if err := host.Connect(ctx, info); err != nil {
				log.Errorf("failed to connect to: %s", info.String())
				return err
			}
		}
		if onConnect != nil {
			if err := onConnect(info, alreadyConnected); err != nil {
				log.Errorf("failed to handle connection to: %s", info.String())
				return err
			}
		}
		log.Infof("connected to: %s", info.String())
	}
	return nil
}

func ConnectBootNodes(ctx context.Context, log logrus.FieldLogger, n node.Node, bootAddrs []ma.Multiaddr) error {
	return ConnectStaticPeers(ctx, log, n, bootAddrs, func(peerInfo peer.AddrInfo, alreadyConnected bool) error {
		// protect the peer, we don't want the peer-limit to mess with the bootnodes when pruning.
		n.Host().ConnManager().Protect(peerInfo.ID, "bootnode")
		log.Infof("added node with bootnode protection: %s", peerInfo.ID.Pretty())
		return nil
	})
}

func ParseMultiAddrs(addrs... string) ([]ma.Multiaddr, error) {
	multiAddrs := make([]ma.Multiaddr, 0, len(addrs))
	for _, addr := range addrs {
		muAddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			return nil, err
		}
		multiAddrs = append(multiAddrs, muAddr)
	}
	return multiAddrs, nil
}
