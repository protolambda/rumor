package base

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/protolambda/rumor/chain"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/rumor/p2p/gossip"
	"github.com/protolambda/rumor/p2p/peering/dv5"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/sirupsen/logrus"
	"net"
	"sync"
)

type Base struct {
	*Actor
	Log    logrus.FieldLogger
}

type Actor struct {
	// only one routine can modify the host at a time
	hLock   sync.Mutex
	p2pHost host.Host

	PrivKey crypto.PrivKey

	IP      net.IP
	TcpPort uint16
	UdpPort uint16

	GlobalPeerInfos   track.PeerInfos
	PeerStatusState   PeerStatusState
	PeerMetadataState PeerMetadataState

	GlobalChains chain.Chains
	ChainState   ChainState

	Dv5State    Dv5State
	GossipState GossipState
	RPCState    RPCState

	ActorCtx    context.Context
	actorCancel context.CancelFunc
}

type PeerStatusState struct {
	Following bool
	Local     methods.Status
}

type PeerMetadataState struct {
	Following bool
	Local     methods.MetaData
}

type ChainState struct {
	CurrentChain chain.ChainID
}

type Dv5State struct {
	Dv5Node dv5.Discv5
}

type GossipState struct {
	GsNode  gossip.GossipSub
	CloseGS context.CancelFunc
	// string -> *pubsub.Topic
	Topics sync.Map
}


func NewActor() *Actor {
	ctxAll, cancelAll := context.WithCancel(context.Background())
	return &Actor{
		ActorCtx:    ctxAll,
		actorCancel: cancelAll,
	}
}

func (r *Actor) Close() {
	r.actorCancel()
}

// shortcut to check if there is a libp2p host available, and error-log if not available.
func (r *Actor) Host() (h host.Host, err error) {
	h = r.p2pHost
	if h == nil {
		return nil, errors.New("REPL must have initialized Libp2p host. Try 'host start'")
	}
	return h, nil
}

func (r *Actor) SetHost(h host.Host) error {
	r.hLock.Lock()
	defer r.hLock.Unlock()
	if r.p2pHost != nil {
		return errors.New("existing host, cannot change host before closing old host")
	}
	r.p2pHost = h
}

func (r *Actor) CloseHost() error {
	r.hLock.Lock()
	defer r.hLock.Unlock()
	if r.p2pHost == nil {
		return errors.New("no host was open")
	}
	err := r.p2pHost.Close()
	if err != nil {
		return err
	}
	r.p2pHost = nil
}

func (r *Actor) GetEnr() *enr.Record {
	priv := (*ecdsa.PrivateKey)(r.PrivKey.(*crypto.Secp256k1PrivateKey))
	return addrutil.MakeENR(r.IP, r.TcpPort, r.UdpPort, priv)
}
