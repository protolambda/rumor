package dv5

import (
	"crypto/ecdsa"
	geth_log "github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/sirupsen/logrus"
	"net"
)

type Discv5 interface {
	Self() *enode.Node
	Lookup(target enode.ID) []*enode.Node
	RandomNodes() enode.Iterator
	Close()
	Ping(n *enode.Node) error
	Resolve(n *enode.Node) *enode.Node
	RequestENR(n *enode.Node) (*enode.Node, error)
}

type Dv5Settings interface {
	LocalNode() *enode.LocalNode
	StaticIP() net.IP
	SetStaticIP(ip net.IP)
	FallbackIP() net.IP
	SetFallbackIP(ip net.IP)
	FallbackUDP() uint16
	SetFallbackUDP(port uint16)
	GetPriv() *crypto.Secp256k1PrivateKey
	SetIP(ip net.IP)
	SetTCP(port uint16)
	SetUDP(port uint16)
	SetEth2Data(dat *beacon.Eth2Data)
	SetAttnets(dat *beacon.AttnetBits)
}

type Discv5Impl struct {
	*discover.UDPv5
	addr *net.UDPAddr
	log  logrus.FieldLogger
	gLog *GethLogger
}

func NewDiscV5(log logrus.FieldLogger, listenIP net.IP, listenPort uint16,
	settings Dv5Settings, bootNodes []*enode.Node) (Discv5, error) {

	priv := settings.GetPriv()

	ecdsaPrivKey := (*ecdsa.PrivateKey)(priv)

	udpAddr := &net.UDPAddr{
		IP:   listenIP,
		Port: int(listenPort),
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	gethLogWrap := GethLogger{FieldLogger: log}
	gethLogger := geth_log.New()
	gethLogger.SetHandler(&gethLogWrap)

	cfg := discover.Config{
		PrivateKey:   ecdsaPrivKey,
		NetRestrict:  nil,
		Bootnodes:    bootNodes,
		Unhandled:    nil, // Not used in dv5
		Log:          gethLogger,
		ValidSchemes: enode.ValidSchemes,
	}
	udpV5, err := discover.ListenV5(conn, settings.LocalNode(), cfg)
	if err != nil {
		return nil, err
	}

	return &Discv5Impl{
		UDPv5: udpV5,
		addr:  udpAddr,
		gLog:  &gethLogWrap,
	}, nil
}
