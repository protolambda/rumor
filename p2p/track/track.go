package track

import (
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"github.com/ethereum/go-ethereum/p2p/enode"
	ds "github.com/ipfs/go-datastore"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"time"
)

var eth2Base = ds.NewKey("/eth2")

type ExtendedPeerstore interface {
	peerstore.Peerstore
	StatusBook
	MetadataBook
	ENRBook
	// TODO: maybe track when we've last seen a peer?
}

type DsExtendedPeerstore struct {
	peerstore.Peerstore
	dsStatusBook
	dsMetadataBook
	dsENRBook
}

var _ IdentifyBook = (*DsExtendedPeerstore)(nil)

func (ep *DsExtendedPeerstore) ProtocolVersion(id peer.ID) (string, error) {
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

func (ep *DsExtendedPeerstore) UserAgent(id peer.ID) (string, error) {
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

type PeerInfoData struct {
	PeerID peer.ID   `json:"peer_id"`
	NodeID enode.ID `json:"node_id"`
	Pubkey string    `json:"pubkey"`

	Addrs     []ma.Multiaddr `json:"addrs"`
	Protocols []string  `json:"protocols"`

	Latency time.Duration `json:"latency"`

	UserAgent       string `json:"user_agent"`
	ProtocolVersion string `json:"protocol_version"`

	// Metadata with highest sequence number
	MetaData *MetaData `json:"metadata"`
	// Highest claimed seq nr, we may not have the actual corresponding metadata yet.
	ClaimedSeq methods.SeqNr `json:"claimed_seq"`
	// Latest status
	Status *Status `json:"status"`
	// Latest ENR
	ENR *enode.Node `json:"enr"`
}

func (ep *DsExtendedPeerstore) GetAllPeerdata(id peer.ID) *PeerInfoData {
	pub := ep.PubKey(id)
	secpKey := (pub).(*ic.Secp256k1PublicKey)
	keyBytes, err := secpKey.Raw()
	pubStr := ""
	if err == nil {
		pubStr = hex.EncodeToString(keyBytes[:])
	}

	nodeID := enode.PubkeyToIDV4((*ecdsa.PublicKey)(secpKey))
	protocols, _ := ep.GetProtocols(id)
	userAgent, _ := ep.UserAgent(id)
	protVersion, _ := ep.ProtocolVersion(id)

	return &PeerInfoData{
		PeerID:             id,
		NodeID:             nodeID,
		Pubkey:             pubStr,
		Addrs:              ep.Addrs(id),
		Protocols:          protocols,
		Latency:            ep.LatencyEWMA(id),
		UserAgent:          userAgent,
		ProtocolVersion:    protVersion,
		MetaData:           ep.Metadata(id),
		ClaimedSeq:         ep.ClaimedSeq(id),
		Status:             ep.Status(id),
		ENR:                ep.LatestENR(),
	}
}
