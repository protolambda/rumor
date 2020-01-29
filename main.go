package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/golang/snappy"
	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
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
		"blocks":             "/eth2/beacon_block/ssz",
		"aggregate":          "/eth2/beacon_aggregate_and_proof/ssz",
		"legacy_attestation": "/eth2/beacon_attestation/ssz",
		"voluntary_exit":     "/eth2/voluntary_exit/ssz",
		"proposer_slashing":  "/eth2/proposer_slashing/ssz",
		"attester_slashing":  "/eth2/attester_slashing/ssz",
		// TODO make this configurable
	}
	for i := 0; i < 8; i++ {
		topics[fmt.Sprintf("committee_%d", i)] = fmt.Sprintf("/eth2/committee_index%d_beacon_attestation/ssz", i)
	}

	ctx, cancel := context.WithCancel(context.Background())

	logger := log.Root()
	logger.SetHandler(log.StreamHandler(os.Stdout, log.TerminalFormat(true)))
	lu, err := NewLurker(ctx, logger)
	if err != nil {
		panic(err)
	}

	outPath := "data"

	if err := lu.StartPubSub(); err != nil {
		panic(err)
	}

	lurkAndLog := func(ctx context.Context, outName string, topic string) error {
		out, err := os.OpenFile(path.Join(outPath, outName+".txt"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, os.ModePerm)
		if err != nil {
			return err
		}
		go func() {
			ticker := time.NewTicker(time.Second * 60)
			for {
				select {
				case <-ticker.C:
					if err := out.Sync(); err != nil {
						lu.log.Error(fmt.Sprintf("Synced %s storage with error: %v", outName, err))
					}
				case <-ctx.Done():
					if err := out.Close(); err != nil {
						lu.log.Error(fmt.Sprintf("Closed %s storage with error: %v", outName, err))
					}
					return
				}
			}
		}()
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

	lu.peerInfoLoop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT)

	select {
	case <-stop:
		log.Info("Exiting...")
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

	peerInfoLog log.Logger
	peerInfoCtx *ManagedCtx

	host    host.Host
	hostCtx *ManagedCtx

	ps    *pubsub.PubSub
	psCtx *ManagedCtx
	psLog log.Logger

	connectionsCtx *ManagedCtx

	kDht    *kad_dht.IpfsDHT
	kDhtCtx *ManagedCtx
	kdLog   log.Logger

	dv5Net *discv5.Network
	dv5Ctx *ManagedCtx
	dv5Log log.Logger
}

func NewLurker(ctx context.Context, l log.Logger) (*Lurker, error) {
	ctx, cancel := context.WithCancel(ctx)
	lu := &Lurker{
		log:        l,
		ManagedCtx: ManagedCtx{ctx: ctx, cancel: cancel},
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
		libp2p.ConnectionManager(connmgr.NewConnManager(15, 20, time.Second*20)),
	}
	lu.hostCtx = lu.NewSubCtx()
	h, err := libp2p.New(lu.hostCtx.ctx, hostOptions...)
	lu.host = h
	lu.log.Info("opened host")
	return err
}

func (lu *Lurker) peerInfoLoop() {
	lu.peerInfoCtx = lu.NewSubCtx()
	lu.peerInfoLog = lu.log
	go func() {
		ticker := time.NewTicker(time.Second * 60)
		end := lu.peerInfoCtx.ctx.Done()
		strAddrs := func(peerID peer.ID) string {
			out := ""
			for _, a := range lu.host.Peerstore().Addrs(peerID) {
				out += " "
				out += a.String()
			}
			return out
		}
		for {
			select {
			case <-ticker.C:
				peers := lu.host.Peerstore().Peers()
				lu.peerInfoLog.Info(fmt.Sprintf("Peerstore size: %d", peers.Len()))
				for i, peerID := range peers {
					lu.peerInfoLog.Trace(fmt.Sprintf(" %d id: %x  %s", i, peerID, strAddrs(peerID)))
				}
				connPeers := lu.host.Network().Peers()
				lu.peerInfoLog.Info(fmt.Sprintf("Network peers size: %d", len(connPeers)))
				for i, peerID := range connPeers {
					lu.peerInfoLog.Trace(fmt.Sprintf(" %d id: %x", i, peerID))
				}
				conns := lu.host.Network().Conns()
				lu.peerInfoLog.Info(fmt.Sprintf("Connections count: %d", len(conns)))
				for i, conn := range conns {
					lu.peerInfoLog.Trace(fmt.Sprintf(" %d id: %x  %s", i, conn.RemotePeer(), conn.RemoteMultiaddr()))
				}
			case <-end:
				lu.peerInfoLog.Info("stopped logging peer info")
				return
			}
		}
	}()
}

func (lu *Lurker) AddKadBootNodes(bootAddrs []ma.Multiaddr) error {
	return lu.ConnectStaticPeers(bootAddrs, func(peerInfo peer.AddrInfo, alreadyConnected bool) error {
		// protect the peer, we don't want the peer-limit to mess with the bootnodes when pruning.
		lu.host.ConnManager().Protect(peerInfo.ID, "bootnode")
		lu.kdLog.Info("added node with bootnode protection: " + peerInfo.ID.String())
		return nil
	})
}

func (lu *Lurker) InitKadDHT(id protocol.ID) error {
	// example protocol id: "/prysm/0.0.0/dht"
	dhtOpts := []dhtopts.Option{
		dhtopts.Datastore(ds_sync.MutexWrap(ds.NewMapDatastore())), // instead of the default map datastore.
		dhtopts.Protocols(id), // don't creep onto the default IPFS network, join the configured DHT
	}
	kdCtx := lu.NewSubCtx()
	kd, err := kad_dht.New(kdCtx.ctx, lu.host, dhtOpts...)
	if err != nil {
		return err
	}
	lu.kDhtCtx = kdCtx
	lu.kDht = kd
	lu.kdLog = lu.log
	lu.kdLog.Info("started KadDHT, protocol: " + string(id))
	return nil
}

func (lu *Lurker) RefresKadTable() {
	refResult := lu.kDht.RefreshRoutingTable()

	// Result is safe to ignore but interesting to log.
	go func() {
		err := <-refResult
		if err != nil {
			lu.kdLog.Error(fmt.Sprintf("failed to refresh kad dht table: %v", err))
		} else {
			lu.kdLog.Info("successfully refreshed kad dht table")
		}
	}()
}

func (lu *Lurker) InitDiscV5(addr string, privKey *ecdsa.PrivateKey) error {
	dv5Log := lu.log

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
		dv5Log.Info("closing discv5", addr)
		dv5Net.Close()
		dv5Log.Info("closed discv5", addr)
	}()

	lu.dv5Net = dv5Net
	lu.dv5Ctx = dv5Ctx
	lu.dv5Log = dv5Log
	return nil
}

func (lu *Lurker) AddDiscV5BootNodes(bootNodes []*discv5.Node) error {
	for _, v := range bootNodes {
		lu.dv5Log.Info("adding discv5 bootnode: ", v.String())
	}
	return lu.dv5Net.SetFallbackNodes(bootNodes)
}

// TODO: implement discv5 polling routine to onboard new peers from into libp2p

func (lu *Lurker) StartWatchingPeers() {
	// TODO: log peers
}

// TODO: re-establish connections with disconnected peers

// TODO: prune peers

// TODO: log metrics of connected/disconnected/etc.

type Compression interface {
	Decompress(r io.Reader) io.Reader
	Compress(w io.WriteCloser) io.WriteCloser
}

type SnappyCompression struct{}

func (c *SnappyCompression) Decompress(reader io.Reader) io.Reader {
	return snappy.NewReader(reader)
}

func (c *SnappyCompression) Compress(w io.WriteCloser) io.WriteCloser {
	return snappy.NewBufferedWriter(w)
}

// ResponseChunkHandler is a function that processes a response chunk. The index, size and result-code are already parsed.
// The contents (decompressed if previously compressed) can be read from r. Optionally an answer can be written back to w.
// If the response chunk could not be processed, an error may be returned.
type ResponseChunkHandler func(ctx context.Context, chunkIndex uint64, chunkSize uint64, result uint8, r io.Reader, w io.Writer) error

// ResponseHandler processes a response, by internally processing chunks, and then reports how many were processed, and if there was an error.
type ResponseHandler func(ctx context.Context, r io.Reader, w io.WriteCloser) (chunkCount uint64, err error)

// makeResponseHandler builds a ResponseHandler, which won't take more than maxChunkCount chunks, or chunks larger than maxChunkSize.
// Compression is optional and may be nil. Chunks are processed by the given ResponseChunkHandler.
func (lu *Lurker) makeResponseHandler(maxChunkCount uint64, maxChunkSize uint64, comp Compression, handle ResponseChunkHandler) ResponseHandler {
	//		response  ::= <response_chunk>*
	//		response_chunk  ::= <result> | <encoding-dependent-header> | <encoded-payload>
	//		result    ::= “0” | “1” | “2” | [“128” ... ”255”]
	return func(ctx context.Context, r io.Reader, w io.WriteCloser) (chunkCount uint64, err error) {
		for chunkIndex := uint64(0); chunkIndex < maxChunkCount; chunkIndex++ {
			resByte := [1]byte{}
			_, err := r.Read(resByte[:])
			if err == io.EOF { // no more chunks left.
				return chunkIndex, nil
			}
			if err != nil {
				return chunkIndex, fmt.Errorf("failed to read chunk %d result byte: %v", chunkIndex, err)
			}
			chunkSize, err := binary.ReadVarint(bufio.NewReader(r)) // TODO unsigned or signed var int?
			if err != nil {
				// TODO send error back: invalid chunk size encoding
				return chunkIndex, err
			}
			if chunkSize < 0 || uint64(chunkSize) > maxChunkSize {
				// TODO sender error back: invalid chunk size, too large.
				return chunkIndex, fmt.Errorf("chunk size %d of chunk %d exceeds chunk limit %d", chunkSize, chunkIndex, maxChunkSize)
			}
			cr := r
			cw := w
			if comp != nil {
				cr = comp.Decompress(cr)
				cw = comp.Compress(cw)
			}
			if err := handle(ctx, chunkIndex, uint64(chunkSize), resByte[0], cr, cw); err != nil {
				return chunkIndex, err
			}
			if comp != nil {
				if err := cw.Close(); err != nil {
					return chunkIndex, fmt.Errorf("failed to close response writer for chunk")
				}
			}
		}
		return maxChunkCount, fmt.Errorf("reached maximum chunk count: %d", maxChunkCount)
	}
}

type responseChunkBuf bytes.Buffer

func (rcb *responseChunkBuf) Close() error {
	return nil
}

func (rcb *responseChunkBuf) Write(p []byte) (n int, err error) {
	return (*bytes.Buffer)(rcb).Write(p)
}

func (rcb *responseChunkBuf) OutputSizeVarint(w io.Writer) error {
	size := (*bytes.Buffer)(rcb).Len()
	sizeBytes := [binary.MaxVarintLen64]byte{}
	// TODO unsigned or signed var int?
	sizeByteLen := binary.PutVarint(sizeBytes[:], int64(size))
	_, err := w.Write(sizeBytes[:sizeByteLen])
	return err
}

func (rcb *responseChunkBuf) WriteTo(w io.Writer) (n int64, err error) {
	return (*bytes.Buffer)(rcb).WriteTo(w)
}

// ResponseChunkWriter reads (decompressed) response message from the msg io.Reader, and writes it as a chunk with given result code to the output writer.
type ResponseChunkWriter func(msg io.Reader, result uint8, w io.Writer) error

// makeResponseChunkWriter builds a ResponseChunkWriter, to write responses. The compression is optional and may be nil.
func makeResponseChunkWriter(comp Compression) ResponseChunkWriter {
	return func(msg io.Reader, result uint8, w io.Writer) error {
		if _, err := w.Write([]byte{result}); err != nil {
			return err
		}
		var out io.Writer
		var buf responseChunkBuf
		out = &buf
		if comp != nil {
			compressedWriter := comp.Compress(&buf)
			defer compressedWriter.Close()
			out = compressedWriter
		}
		if _, err := io.Copy(out, msg); err != nil {
			return err
		}
		if _, err := w.Write([]byte{result}); err != nil {
			return err
		}
		if err := buf.OutputSizeVarint(w); err != nil {
			return err
		}
		if _, err := buf.WriteTo(w); err != nil {
			return err
		}
		return nil
	}
}

// RequestHandler processes a request (decompressed if previously compressed), read from r.
// The handler can respond by writing to w. After returning the writer will automatically be closed.
type RequestHandler func(ctx context.Context, r io.Reader, w io.Writer)

// startReqRPC registers a request handler for the given protocol. Compression is optional and may be nil.
func (lu *Lurker) startReqRPC(id protocol.ID, comp Compression, handle RequestHandler) {
	logger := lu.log
	lu.host.SetStreamHandler(id, func(stream network.Stream) {
		defer stream.Close()
		deadlineDuration := time.Second * 2 // TODO decide on incoming stream timeout
		ctx, cancel := context.WithTimeout(lu.NewSubCtx().ctx, deadlineDuration)
		defer cancel()
		if err := stream.SetReadDeadline(time.Now().Add(deadlineDuration)); err != nil {
			logger.Error(err.Error())
		}
		r := io.Reader(stream)
		w := io.WriteCloser(stream)
		if comp != nil {
			r = comp.Decompress(r)
			w = comp.Compress(w)
			defer w.Close()
		}
		handle(ctx, r, w)
	})
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
	return &discv5.Node{IP: ip, UDP: udpPort, TCP: tcpPort, ID: id}, nil
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
		alreadyConnected := len(lu.host.Network().ConnsToPeer(info.ID)) > 0
		if !alreadyConnected {
			peerCtx := lu.connectionsCtx.NewSubCtx()
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
	lu.psLog = lu.log
	lu.psLog.Info("started pubsub")
	return nil
}

type PubSubMessage struct {
	from peer.ID
	data []byte
}

func (lu *Lurker) LurkTopic(ctx context.Context, topic string, out chan<- PubSubMessage, outErr chan<- error) (err error) {
	lu.psLog.Info("listening on pubsub topic: " + topic)
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
		lu.log.Info("start subscription on topic " + topic)
		for {
			msg, err := sub.Next(lu.ctx)
			if !running {
				break
			}
			if err != nil {
				outErr <- err
				continue
			}
			lu.psLog.Debug(fmt.Sprintf("Received message on '%s' from %s: %d bytes, seq nr: %x", topic, msg.GetFrom().Pretty(), msg.Size(), msg.Seqno))
			out <- PubSubMessage{
				from: msg.GetFrom(),
				data: msg.Data,
			}
		}
		lu.log.Info("stopped subscription on topic " + topic)
	}()

	go func() {
		<-ctx.Done()
		lu.psLog.Info("stopping listening on topic: " + topic)
		running = false
	}()

	return nil
}

func NewMessageLogger(ctx context.Context, w io.Writer, outErr chan<- error) chan<- PubSubMessage {
	data := make([]byte, 1<<14)
	outErr = make(chan error)
	onMsg := func(msg PubSubMessage) {
		n := hex.EncodedLen(len(msg.data))
		if n > len(data) {
			data = make([]byte, n*2, n*2)
		}
		hex.Encode(data, msg.data)
		if _, err := w.Write(data[:n]); err != nil {
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
	l := lu.log
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
