package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	kad_dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-pubsub"
	ma "github.com/multiformats/go-multiaddr"
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

	//ownStatus *Status
	//peers     map[peer.ID]*Status

	rpcLog log.Logger
}

func NewLurker(ctx context.Context, l log.Logger) (*Lurker, error) {
	ctx, cancel := context.WithCancel(ctx)
	lu := &Lurker{
		log:        l,
		ManagedCtx: ManagedCtx{ctx: ctx, cancel: cancel},
	}
	lu.rpcLog = l // TODO set up topic based logging
	return lu, lu.openHost()
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

// TODO: implement discv5 polling routine to onboard new peers from into libp2p

func (lu *Lurker) StartWatchingPeers() {
	// TODO: log peers
}

// TODO: re-establish connections with disconnected peers

// TODO: prune peers

// TODO: log metrics of connected/disconnected/etc.


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
