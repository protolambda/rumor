package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"io"
	"log"
	"path"
	//"github.com/libp2p/go-libp2p-secio"
	"github.com/libp2p/go-tcp-transport"
	"github.com/minio/sha256-simd"
	ma "github.com/multiformats/go-multiaddr"
	"os"
	"os/signal"
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
		errLogger := NewErrLogger(ctx, outName, os.Stderr)
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
	if err := lu.ConnectStaticPeers([]ma.Multiaddr{
		// TODO connect with peers
	}); err != nil {
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

	host    host.Host
	hostCtx *ManagedCtx

	ps    *pubsub.PubSub
	psCtx *ManagedCtx

	connectionsCtx *ManagedCtx
	connections    map[peer.ID]*ManagedCtx
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

// TODO add KDHT and discV5 for more peers

func (lu *Lurker) ConnectStaticPeers(multiAddrs []ma.Multiaddr) error {
	infos, err := peer.AddrInfosFromP2pAddrs(multiAddrs...)
	if err != nil {
		return err
	}
	if lu.connectionsCtx == nil {
		lu.connectionsCtx = lu.NewSubCtx()
	}
	for _, info := range infos {
		peerCtx := lu.connectionsCtx.NewSubCtx()
		lu.connections[info.ID] = peerCtx
		if err := lu.host.Connect(peerCtx.ctx, info); err != nil {
			return err
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

func NewErrLogger(ctx context.Context, name string, w io.Writer) chan<- error {
	l := log.New(w, name+": ", log.LUTC|log.Ldate)
	listenerCh := make(chan error)
	go func() {
		for {
			select {
			case msg := <-listenerCh:
				l.Println(msg)
			case <-ctx.Done():
				return
			}
		}
	}()
	return listenerCh
}
