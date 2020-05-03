package static

import (
	"context"
	core "github.com/libp2p/go-libp2p-core"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
)

type ConnectionCallback func(info peer.AddrInfo, alreadyConnected bool) error

// onConnect may be nil, if no further action is required after starting the connection.
func ConnectStaticPeers(ctx context.Context, log logrus.FieldLogger, host core.Host, multiAddrs []ma.Multiaddr, onConnect ConnectionCallback) error {
	infos, err := peer.AddrInfosFromP2pAddrs(multiAddrs...)
	if err != nil {
		log.Errorf("failed to read multi addrs: %v", multiAddrs)
		return err
	}
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

func ParseMultiAddrs(addrs ...string) ([]ma.Multiaddr, error) {
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
