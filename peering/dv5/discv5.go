package dv5

import (
	"context"
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
	ReadRandomNodes(buf []*enode.Node) int
	Resolve(n *enode.Node) *enode.Node
	RequestENR(n *enode.Node) (*enode.Node, error)
	Trace(v bool)
}

type Discv5Impl struct {
	*discover.UDPv5
	addr  *net.UDPAddr
	log   logrus.FieldLogger
	gLog *GethLogger
}

func (d Discv5Impl) Trace(v bool) {
	if v {
		d.gLog.LogLevel = geth_log.LvlTrace
	} else {
		d.gLog.LogLevel = geth_log.LvlInfo
	}
}

type GethLogger struct {
	logrus.FieldLogger
	LogLevel geth_log.Lvl
}

// New returns a new Logger that has this logger's context plus the given context
func (l *GethLogger) Log(r *geth_log.Record) error {
	if l.LogLevel < r.Lvl {
		return nil
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

func NewDiscV5(ctx context.Context, n node.Node, ip net.IP, port uint16, privKey crypto.PrivKey, bootNodes []*enode.Node) (Discv5, error) {
	dv5Log := n.Logger("discv5")
	k, ok := privKey.(*crypto.Secp256k1PrivateKey)
	if !ok {
		return nil, errors.New("libp2p-crypto private key is not a Secp256k1 key")
	}
	ecdsaPrivKey := (*ecdsa.PrivateKey)((*btcec.PrivateKey)(k))

	udpAddr := &net.UDPAddr{
		IP:   ip,
		Port: int(port),
	}

	dv5Log = dv5Log.WithField("addr", udpAddr)

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		dv5Log.Debugf("UDP listener start err: %v", err)
		return nil, err
	}
	dv5Log.Debug("UDP listener up")

	localNodeDB, err := enode.OpenDB("") // memory-DB
	localNode := enode.NewLocalNode(localNodeDB, ecdsaPrivKey)
	localNode.SetFallbackIP(udpAddr.IP)
	localNode.SetFallbackUDP(udpAddr.Port)

	unhandledMsgs := make(chan discover.ReadPacket)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-unhandledMsgs:
				dv5Log.Debugf("Received unhandled message from %s: %x", msg.Addr.String(), msg.Data)
			}
		}
	}()
	gethLogWrap := GethLogger{FieldLogger: dv5Log, LogLevel: geth_log.LvlWarn}
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
		dv5Log.Debugf("Discv5 listener start err: %v", err)
		return nil, err
	}
	dv5Log.Debug("Discv5 listener up")
	go func() {
		<-ctx.Done()
		dv5Log.Info("closing discv5", udpAddr.String())
		udpV5.Close()
		close(unhandledMsgs)
		dv5Log.Info("closed discv5", udpAddr.String())
	}()

	return &Discv5Impl{
		UDPv5: udpV5,
		addr:  udpAddr,
		log:   dv5Log,
		gLog:  &gethLogWrap,
	}, nil
}

