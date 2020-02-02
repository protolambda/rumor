package node

import (
	"context"
	"crypto/rand"
	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	mplex "github.com/libp2p/go-libp2p-mplex"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	secio "github.com/libp2p/go-libp2p-secio"
	yamux "github.com/libp2p/go-libp2p-yamux"
	"github.com/libp2p/go-tcp-transport"
	"github.com/sirupsen/logrus"
	"time"
)

type Logger interface {
	logrus.FieldLogger
}

type Node interface {
	Host() host.Host
	Logger(logTopic string) logrus.FieldLogger
}

type LocalNode struct {
	ctx context.Context
	host host.Host
	priv crypto.PrivKey
	log logrus.FieldLogger
}

// using defaults
func NewLocalNode(ctx context.Context, log logrus.FieldLogger, options ...libp2p.Option) (*LocalNode, error) {
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, rand.Reader)
	if err != nil {
		return nil, err
	}
	// The eth2 defaults.
	hostOptions := []libp2p.Option{
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Identity(priv),
		// TODO: want to enable this yet?
		//libp2p.Transport(ws.New),
		//libp2p.EnableRelay(),
		libp2p.Peerstore(pstoremem.NewPeerstore()), // TODO: persist peerstore?
		libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport),
		libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport),
		libp2p.Security(secio.ID, secio.New),
		// TODO: time for other security options?
		libp2p.ConnectionManager(connmgr.NewConnManager(15, 20, time.Second*20)),
	}
	hostOptions = append(hostOptions, options...)
	h, err := libp2p.New(ctx, hostOptions...)
	if err != nil {
		return nil, err
	}
	return &LocalNode{
		ctx:  ctx,
		host: h,
		priv: priv,
		log: log,
	}, nil
}

func (l *LocalNode) Host() host.Host {
	return l.host
}

func (l *LocalNode) Logger(logTopic string) logrus.FieldLogger {
	return l.log.WithField("log_topic", logTopic)
}
