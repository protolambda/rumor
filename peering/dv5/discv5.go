package dv5

import (
	"crypto/ecdsa"
	"errors"
	"github.com/btcsuite/btcd/btcec"
	geth_log "github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/protolambda/rumor/node"
	"github.com/sirupsen/logrus"
	"net"
)

type Discv5 interface {
	Self() *enode.Node
	Lookup(target enode.ID) []*enode.Node
	LookupRandom() []*enode.Node
	Close()
	Ping(n *enode.Node) error
	Resolve(n *enode.Node) *enode.Node
	RequestENR(n *enode.Node) (*enode.Node, error)
}

type Discv5Impl struct {
	*discover.UDPv5
	addr *net.UDPAddr
	log  logrus.FieldLogger
	gLog *GethLogger
}

type GethLogger struct {
	logrus.FieldLogger
}

// New returns a new Logger that has this logger's context plus the given context
func (gl *GethLogger) Log(r *geth_log.Record) error {
	rCtx := r.Ctx
	l := gl.FieldLogger
	for i := 0; i < len(rCtx); i += 2 {
		l = l.WithField(rCtx[i].(string), rCtx[i+1])
	}
	switch r.Lvl {
	case geth_log.LvlCrit:
		l.Panicln(r.Msg)
	case geth_log.LvlError:
		l.Errorln(r.Msg)
	case geth_log.LvlWarn:
		l.Warningln(r.Msg)
	case geth_log.LvlInfo:
		l.Infoln(r.Msg)
	case geth_log.LvlDebug:
		l.Debugln(r.Msg)
	case geth_log.LvlTrace:
		l.Debugln(r.Msg)
	}
	return nil
}

func NewDiscV5(log logrus.FieldLogger, n node.Node, ip net.IP, port uint16, privKey crypto.PrivKey, bootNodes []*enode.Node) (Discv5, error) {
	k, ok := privKey.(*crypto.Secp256k1PrivateKey)
	if !ok {
		return nil, errors.New("libp2p-crypto private key is not a Secp256k1 key")
	}
	ecdsaPrivKey := (*ecdsa.PrivateKey)((*btcec.PrivateKey)(k))

	udpAddr := &net.UDPAddr{
		IP:   ip,
		Port: int(port),
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	localNodeDB, err := enode.OpenDB("") // memory-DB
	localNode := enode.NewLocalNode(localNodeDB, ecdsaPrivKey)
	localNode.SetFallbackIP(udpAddr.IP)
	localNode.SetFallbackUDP(udpAddr.Port)

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
	udpV5, err := discover.ListenV5(conn, localNode, cfg)
	if err != nil {
		return nil, err
	}

	return &Discv5Impl{
		UDPv5: udpV5,
		addr:  udpAddr,
		gLog:  &gethLogWrap,
	}, nil
}
