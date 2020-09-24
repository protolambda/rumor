package dstrack

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/p2p/enode"
	ds "github.com/ipfs/go-datastore"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-peerstore/pstoreds"
	"github.com/multiformats/go-base32"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/protolambda/rumor/p2p/track/dstee"
	"github.com/protolambda/zrnt/eth2/beacon"
	"io"
	"sync"
)

var eth2Base = ds.NewKey("/peers/eth2")

func peerIdToKey(base ds.Key, p peer.ID) ds.Key {
	return base.ChildString(base32.RawStdEncoding.EncodeToString([]byte(p)))
}

type dsExtendedPeerstore struct {
	multiTeeLock sync.Mutex
	multiTee     dstee.MultiTee
	store        ds.Batching
	peerstore.Peerstore
	*dsStatusBook
	*dsMetadataBook
	*dsENRBook
}

func NewExtendedPeerstore(ctx context.Context, store ds.Batching, opts pstoreds.Options) (track.ExtendedPeerstore, error) {
	mul := dstee.MultiTee{}
	store = &dstee.DSTee{
		Batching: store,
		Tee:      mul,
	}
	ps, err := pstoreds.NewPeerstore(ctx, store, opts)
	if err != nil {
		return nil, err
	}
	sb, err := NewStatusBook(store)
	if err != nil {
		return nil, err
	}
	mb, err := NewMetadataBook(store)
	if err != nil {
		return nil, err
	}
	eb, err := NewENRBook(store)
	if err != nil {
		return nil, err
	}

	return &dsExtendedPeerstore{
		multiTee:       mul,
		store:          store,
		Peerstore:      ps,
		dsStatusBook:   sb,
		dsMetadataBook: mb,
		dsENRBook:      eb,
	}, nil
}

var _ track.IdentifyBook = (*dsExtendedPeerstore)(nil)

func (ep *dsExtendedPeerstore) Datastore() ds.Batching {
	return ep.store
}

func (ep *dsExtendedPeerstore) AddTee(tee dstee.Tee) (exists bool) {
	ep.multiTeeLock.Lock()
	defer ep.multiTeeLock.Unlock()
	if _, exists := ep.multiTee[tee]; exists {
		return true
	}
	ep.multiTee[tee] = struct{}{}
	return false
}

func (ep *dsExtendedPeerstore) RmTee(tee dstee.Tee) (exists bool) {
	ep.multiTeeLock.Lock()
	defer ep.multiTeeLock.Unlock()
	if _, exists := ep.multiTee[tee]; !exists {
		return false
	}
	delete(ep.multiTee, tee)
	return true
}

func (ep *dsExtendedPeerstore) ListTees() (out []dstee.Tee) {
	ep.multiTeeLock.Lock()
	defer ep.multiTeeLock.Unlock()
	for t := range ep.multiTee {
		out = append(out, t)
	}
	return
}

func (ep *dsExtendedPeerstore) ProtocolVersion(id peer.ID) (string, error) {
	dat, err := ep.Get(id, "ProtocolVersion")
	if err != nil {
		return "", err
	}
	v, ok := dat.(string)
	if !ok {
		return "", errors.New("protocol version is not a string")
	}
	return v, nil
}

func (ep *dsExtendedPeerstore) UserAgent(id peer.ID) (string, error) {
	dat, err := ep.Get(id, "AgentVersion") // actually called AgentVersion in store.
	if err != nil {
		return "", err
	}
	v, ok := dat.(string)
	if !ok {
		return "", errors.New("user agent is not a string")
	}
	return v, nil
}

type Flusher interface {
	flush() error
}

// TODO: start this as a background service, similar to the GC of addrs in the peerstore.
func (ep *dsExtendedPeerstore) flush() error {
	var errs []error
	weakFlush := func(name string, c interface{}) {
		if cl, ok := c.(Flusher); ok {
			if err := cl.flush(); err != nil {
				errs = append(errs, fmt.Errorf("%s error: %s", name, err))
			}
		}
	}
	weakFlush("statusbook", ep.dsStatusBook)
	weakFlush("metadatabook", ep.dsMetadataBook)
	weakFlush("enrbook", ep.dsENRBook)

	if len(errs) > 0 {
		return fmt.Errorf("failed while flushing peerstore data; err(s): %q", errs)
	}
	return nil
}

func (ep *dsExtendedPeerstore) Close() error {
	var errs []error
	weakClose := func(name string, c interface{}) {
		if cl, ok := c.(io.Closer); ok {
			if err := cl.Close(); err != nil {
				errs = append(errs, fmt.Errorf("%s error: %s", name, err))
			}
		}
	}
	weakClose("inner", ep.Peerstore)
	weakClose("statusbook", ep.dsStatusBook)
	weakClose("metadatabook", ep.dsMetadataBook)
	weakClose("enrbook", ep.dsENRBook)

	if len(errs) > 0 {
		return fmt.Errorf("failed while closing peerstore; err(s): %q", errs)
	}
	return nil
}

func (ep *dsExtendedPeerstore) GetAllData(id peer.ID) *track.PeerAllData {
	pub := ep.PubKey(id)
	secpKey := (pub).(*ic.Secp256k1PublicKey)
	keyBytes, err := secpKey.Raw()
	pubStr := ""
	if err == nil {
		pubStr = hex.EncodeToString(keyBytes[:])
	}

	nodeID := enode.PubkeyToIDV4((*ecdsa.PublicKey)(secpKey))
	protocols, err := ep.GetProtocols(id)
	if err != nil {
		fmt.Printf("couldn't get protocols: %v\n", err)
	}
	userAgent, _ := ep.UserAgent(id)
	protVersion, _ := ep.ProtocolVersion(id)
	seq, _ := ep.ClaimedSeq(id)
	var multiAddrs []string
	for _, addr := range ep.Addrs(id) {
		multiAddrs = append(multiAddrs, addr.String())
	}
	var enrAttnets *beacon.AttnetBits

	var forkDigest *beacon.ForkDigest
	var nextForkVersion *beacon.Version
	var nextForkEpoch *beacon.Epoch

	en := ep.LatestENR(id)
	if en != nil {
		if dat, exists, err := addrutil.ParseEnrEth2Data(en); err == nil && exists {
			forkDigest = &dat.ForkDigest
			nextForkVersion = &dat.NextForkVersion
			nextForkEpoch = &dat.NextForkEpoch
		}
		if dat, exists, err := addrutil.ParseEnrAttnets(en); err == nil && exists {
			enrAttnets = dat
		}
	}
	return &track.PeerAllData{
		PeerID:          id,
		NodeID:          nodeID,
		Pubkey:          pubStr,
		Addrs:           multiAddrs,
		Protocols:       protocols,
		Latency:         ep.LatencyEWMA(id),
		UserAgent:       userAgent,
		ProtocolVersion: protVersion,
		ForkDigest:      forkDigest,
		NextForkVersion: nextForkVersion,
		NextForkEpoch:   nextForkEpoch,
		Attnets:         enrAttnets,
		MetaData:        ep.Metadata(id),
		ClaimedSeq:      seq,
		Status:          ep.Status(id),
		ENR:             en,
	}
}
