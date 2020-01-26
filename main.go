package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/rlp"
	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	kad_dht "github.com/libp2p/go-libp2p-kad-dht"
	dhtopts "github.com/libp2p/go-libp2p-kad-dht/opts"
	"github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-tcp-transport"
	"github.com/minio/sha256-simd"
	ma "github.com/multiformats/go-multiaddr"
	"io"
	"net"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"
)

func main() {
	topics := map[string]string{
		"blocks": "/eth2/beacon_block/ssz",
		// TODO make this configurable
	}

	ctx, cancel := context.WithCancel(context.Background())

	lu, err := NewLurker(ctx)
	if err != nil {
		panic(err)
	}

	outPath := "logs"

	if err := lu.StartPubSub(); err != nil {
		panic(err)
	}

	lurkAndLog := func(ctx context.Context, outName string, topic string) error {
		out, err := os.OpenFile(path.Join(outPath, outName), os.O_WRONLY|os.O_CREATE|os.O_APPEND, os.ModePerm)
		if err != nil {
			return err
		}
		errLogger := lu.NewErrLogger(ctx, outName)
		msgLogger := NewMessageLogger(ctx, out, errLogger)
		return lu.LurkTopic(ctx, topic, msgLogger, errLogger)
	}

	for name, top := range topics {
		if err := lurkAndLog(ctx, name, top); err != nil {
			panic(fmt.Errorf("topic %s failed to start running: %v", name, err))
		}
	}

	// Connect with peers after the pubsub is all set up,
	// so that peers do not have to learn about pubsub interest after being connected.

	// TODO: enode list option -> bootnodes discv5
	// TODO: multi addr list option -> bootnodes kdht

	// kademlia
	{
		if err := lu.InitKadDHT("/prysm/0.0.0/dht"); err != nil {
			panic(err)
		}
		bootAddrStrs := []string{
			"/dns4/prylabs.net/tcp/30001/p2p/16Uiu2HAm7Qwe19vz9WzD2Mxn7fXd1vgHHp4iccuyq7TxwRXoAGfc",
		}
		bootNodes := make([]ma.Multiaddr, 0, len(bootAddrStrs))
		for _, addr := range bootAddrStrs {
			muAddr, err := ma.NewMultiaddr(addr)
			if err != nil {
				panic(err)
			}
			bootNodes = append(bootNodes, muAddr)
		}
		if err := lu.AddKadBootNodes(bootNodes); err != nil {
			panic(err)
		}
	}

	// disc v5
	if false { // disabled for now
		// TODO configure udp addr and keypair
		if err := lu.InitDiscV5("", nil); err != nil {
			panic(err)
		}
		bootAddrStrs := []string{
			".........", // TODO base64 enr with udp port
		}
		bootNodes := make([]*discv5.Node, 0, len(bootAddrStrs))
		for _, addr := range bootAddrStrs {
			enrRec, err := parseEnr(addr)
			if err != nil {
				panic(err)
			}
			enodeAddr, err := enrToEnode(enrRec, true)
			if err != nil {
				panic(err)
			}
			dv5Node, err := enodeToDiscv5Node(enodeAddr)
			if err != nil {
				panic(err)
			}
			bootNodes = append(bootNodes, dv5Node)
		}
		if err := lu.AddDiscV5BootNodes(bootNodes); err != nil {
			panic(err)
		}
	}

	// static peers
	if err := lu.ConnectStaticPeers([]ma.Multiaddr{
		// TODO connect with peers
	}, nil); err != nil {
		panic(err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT)

	select {
	case <-stop:
		cancel()
		time.Sleep(time.Second)
		os.Exit(0)
	}
}

func msgIDFunction(pmsg *pubsub_pb.Message) string {
	h := sha256.New()
	// never errors, see crypto/sha256 Go doc
	_, _ = h.Write(pmsg.Data)
	id := h.Sum(nil)
	return base64.URLEncoding.EncodeToString(id)
}

type ManagedCtx struct {
	ctx    context.Context
	cancel func()
}

func (mCtx *ManagedCtx) NewSubCtx() *ManagedCtx {
	ctx, cancel := context.WithCancel(mCtx.ctx)
	return &ManagedCtx{
		ctx:    ctx,
		cancel: cancel,
	}
}

type Lurker struct {
	ManagedCtx

	log log.Logger

	host    host.Host
	hostCtx *ManagedCtx

	ps    *pubsub.PubSub
	psCtx *ManagedCtx

	connectionsCtx *ManagedCtx
	connections    map[peer.ID]*ManagedCtx

	kDht    *kad_dht.IpfsDHT
	kDhtCtx *ManagedCtx

	dv5Net  *discv5.Network
	dv5Ctx  *ManagedCtx
	dv5Log  log.Logger
}

func NewLurker(ctx context.Context) (*Lurker, error) {
	ctx, cancel := context.WithCancel(ctx)
	lu := &Lurker{
		ManagedCtx:     ManagedCtx{ctx: ctx, cancel: cancel},
		connectionsCtx: nil,
		connections:    nil,
	}
	return lu, lu.openHost()
}

func (lu *Lurker) openHost() error {
	hostOptions := []libp2p.Option{
		libp2p.Transport(tcp.NewTCPTransport),
		//libp2p.Transport(ws.New),
		//libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport),
		//libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport),
		//libp2p.Security(secio.ID, secio.New),
	}
	lu.hostCtx = lu.NewSubCtx()
	h, err := libp2p.New(lu.hostCtx.ctx, hostOptions...)
	lu.host = h
	return err
}

func (lu *Lurker) AddKadBootNodes(bootAddrs []ma.Multiaddr) error {
	return lu.ConnectStaticPeers(bootAddrs, func(peerInfo peer.AddrInfo, alreadyConnected bool) error {
		// protect the peer, we don't want the peer-limit to mess with the bootnodes when pruning.
		lu.host.ConnManager().Protect(peerInfo.ID, "bootnode")
		return nil
	})
}

func (lu *Lurker) InitKadDHT(id protocol.ID) error {
	// example protocol id: "/prysm/0.0.0/dht"
	dhtOpts := []dhtopts.Option{
		dhtopts.Datastore(ds_sync.MutexWrap(ds.NewMapDatastore())),  // instead of the default map datastore.
		dhtopts.Protocols(id),  // don't creep onto the default IPFS network, join the configured DHT
	}
	kdCtx := lu.NewSubCtx()
	kd, err := kad_dht.New(lu.kDhtCtx.ctx, lu.host, dhtOpts...)
	if err != nil {
		return err
	}
	lu.kDhtCtx = kdCtx
	lu.kDht = kd
	return nil
}

func (lu *Lurker) RefresKadTable() {
	refResult := lu.kDht.RefreshRoutingTable()

	// Result is safe to ignore but interesting to log.
	go func() {
		err := <-refResult
		if err != nil {
			fmt.Printf("failed to refresh kad dht table: %v", err)
		} else {
			fmt.Println("successfully refreshed kad dht table")
		}
	}()
}

func (lu *Lurker) InitDiscV5(addr string, privKey *ecdsa.PrivateKey) error {
	dv5Log := lu.log.New("discv5")

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	dv5Log.Debug("UDP listener up", "addr", udpAddr)

	dv5Net, err := discv5.ListenUDP(privKey, conn, "", nil)
	if err != nil {
		return err
	}
	dv5Log.Debug("Discv5 listener up", "addr", udpAddr)

	dv5Ctx := lu.NewSubCtx()
	go func() {
		<-dv5Ctx.ctx.Done()
		dv5Net.Close()
	}()

	lu.dv5Net = dv5Net
	lu.dv5Ctx = dv5Ctx
	lu.dv5Log = dv5Log
	return nil
}

func (lu *Lurker) AddDiscV5BootNodes(bootNodes []*discv5.Node) error {
	return lu.dv5Net.SetFallbackNodes(bootNodes)
}

func parseEnr(v string) (*enr.Record, error) {
	data, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return nil, err
	}
	var record enr.Record
	if err := rlp.Decode(bytes.NewReader(data), &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func enrToEnode(record *enr.Record, verifySig bool) (*enode.Node, error) {
	idSchemeName := record.IdentityScheme()

	if verifySig {
		if err := record.VerifySignature(enode.ValidSchemes[idSchemeName]); err != nil {
			return nil, err
		}
	}

	return enode.New(enode.ValidSchemes[idSchemeName], record)
}

func enodeToDiscv5Node(en *enode.Node) (*discv5.Node, error) {
	id := discv5.PubkeyID(en.Pubkey())
	ip := en.IP()
	udpPort, tcpPort := uint16(en.UDP()), uint16(en.TCP())
	if ip == nil || udpPort == 0 || tcpPort == 0 {
		return nil, fmt.Errorf("enode record %v has missing ip/udp/tcp", en.String())
	}
	return &discv5.Node{IP:  ip, UDP: udpPort, TCP: tcpPort, ID:  id}, nil
}

type ConnectionCallback func(info peer.AddrInfo, alreadyConnected bool) error

// onConnect may be nil, if no further action is required after starting the connection.
func (lu *Lurker) ConnectStaticPeers(multiAddrs []ma.Multiaddr, onConnect ConnectionCallback) error {
	infos, err := peer.AddrInfosFromP2pAddrs(multiAddrs...)
	if err != nil {
		return err
	}
	if lu.connectionsCtx == nil {
		lu.connectionsCtx = lu.NewSubCtx()
	}
	for _, info := range infos {
		alreadyConnected := lu.connections[info.ID] != nil
		if !alreadyConnected {
			peerCtx := lu.connectionsCtx.NewSubCtx()
			lu.connections[info.ID] = peerCtx
			if err := lu.host.Connect(peerCtx.ctx, info); err != nil {
				return err
			}
		}
		if onConnect != nil {
			if err := onConnect(info, alreadyConnected); err != nil {
				return err
			}
		}
	}
	return nil
}

func (lu *Lurker) StartPubSub() error {
	psOptions := []pubsub.Option{
		pubsub.WithMessageSigning(false),
		pubsub.WithStrictSignatureVerification(false),
		pubsub.WithMessageIdFn(msgIDFunction),
	}
	lu.psCtx = lu.NewSubCtx()
	ps, err := pubsub.NewGossipSub(lu.psCtx.ctx, lu.host, psOptions...)
	if err != nil {
		return err
	}
	lu.ps = ps
	return nil
}

type PubSubMessage struct {
	from peer.ID
	data []byte
}

func (lu *Lurker) LurkTopic(ctx context.Context, topic string, out chan<- PubSubMessage, outErr chan<- error) (err error) {
	top, err := lu.ps.Join(topic)
	if err != nil {
		return err
	}
	sub, err := top.Subscribe()
	if err != nil {
		return err
	}
	running := true
	go func() {
		for {
			msg, err := sub.Next(lu.ctx)
			if !running {
				break
			}
			if err != nil {
				outErr <- err
				continue
			}
			out <- PubSubMessage{
				from: msg.GetFrom(),
				data: msg.Data,
			}
		}
	}()

	go func() {
		<-ctx.Done()
		running = false
	}()

	return nil
}

func NewMessageLogger(ctx context.Context, w io.Writer, outErr chan<- error) chan<- PubSubMessage {
	var buf bytes.Buffer
	outErr = make(chan error)
	onMsg := func(msg PubSubMessage) {
		buf.Reset()
		buf.Grow(hex.EncodedLen(len(msg.data)))
		hex.Encode(buf.Bytes(), msg.data)
		if _, err := w.Write(buf.Bytes()); err != nil {
			outErr <- err
		}
		if _, err := w.Write([]byte{'\n'}); err != nil {
			outErr <- err
		}
	}

	listenerCh := make(chan PubSubMessage)

	go func() {
		for {
			select {
			case msg := <-listenerCh:
				onMsg(msg)
			case <-ctx.Done():
				return
			}
		}
	}()
	return listenerCh
}

func (lu *Lurker) NewErrLogger(ctx context.Context, name string) chan<- error {
	l := lu.log.New(name)
	listenerCh := make(chan error)
	go func() {
		for {
			select {
			case msg := <-listenerCh:
				l.Error(msg.Error())
			case <-ctx.Done():
				return
			}
		}
	}()
	return listenerCh
}
