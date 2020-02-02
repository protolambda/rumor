package discv5

import (
	"context"
	"crypto/ecdsa"
	"eth2-lurk/node"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/sirupsen/logrus"
	"net"
)

type Discv5 interface {
	AddDiscV5BootNodes(bootNodes []*discv5.Node) error
}

type Discv5Impl struct {
	net *discv5.Network
	log logrus.FieldLogger
}

func NewDiscV5(ctx context.Context, n node.Node, addr string, privKey *ecdsa.PrivateKey) (Discv5, error) {
	dv5Log := n.Logger("discv5")

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	dv5Log = dv5Log.WithField("addr", udpAddr)

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		dv5Log.Debugf("UDP listener start err: %v", err)
		return nil, err
	}
	dv5Log.Debug("UDP listener up")

	dv5Net, err := discv5.ListenUDP(privKey, conn, "", nil)
	if err != nil {
		dv5Log.Debugf("Discv5 listener start err: %v", err)
		return nil, err
	}
	dv5Log.Debug("Discv5 listener up")

	go func() {
		<-ctx.Done()
		dv5Log.Info("closing discv5", addr)
		dv5Net.Close()
		dv5Log.Info("closed discv5", addr)
	}()

	return &Discv5Impl{
		log: dv5Log,
		net:    dv5Net,
	}, nil
}

func (dv5 *Discv5Impl) AddDiscV5BootNodes(bootNodes []*discv5.Node) error {
	for _, v := range bootNodes {
		dv5.log.Info("adding discv5 bootnode: ", v.String())
	}
	return dv5.net.SetFallbackNodes(bootNodes)
}
