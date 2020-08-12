package translate

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
	ds "github.com/ipfs/go-datastore"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	pstore_pb "github.com/libp2p/go-libp2p-peerstore/pb"
	"github.com/multiformats/go-base32"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/protolambda/zssz"
)

/*
Peerstore layout, partially due to libp2p legacy:

	/peers
		- /eth2
		 	- /<peer-id>
				- /metadata           <- ssz encoded
				- /metadata_claim     <- ssz encoded
				- /status             <- ssz encoded
				- /enr                <- base64 enr representation
		- /addrs
			- /<peer-id>              <- no subkeys. Encoded as `pstore_pb.AddrBookRecord` protobuf
		- /metadata
			- /<peer-id>
				- /protocols          <- bare map[string]struct{}, encoded by libp2p, using Gob for encode/decode :(

				- /ProtocolVersion    <- put there sneakily by libp2p identify protocol. Gob encoded string
				- /AgentVersion       <- put there sneakily by libp2p identify protocol. Gob encoded string
				- /<other misc keys>  <- user can add anything
		- /keys
			- /<peer-id>
				- /pub      <- encoded as protobuf by libp2p crypto package
				- /priv     <- encoded as protobuf by libp2p crypto package
*/

type ENRData struct {
	Raw      string            `json:"raw,omitempty"`
	Contents map[string]string `json:"contents,omitempty"`
}

type AddrBookRecord struct {
	// The multiaddresses. This is a sorted list where element 0 expires the soonest.
	Addrs []*AddrBookRecord_AddrEntry `json:"addrs,omitempty"`
	// The most recently received signed PeerRecord.
	CertifiedRecord *AddrBookRecord_CertifiedRecord `json:"certified_record,omitempty"`
}

type AddrBookRecord_AddrEntry struct {
	Addr ma.Multiaddr `json:"addr,omitempty"`
	// The point in time when this address expires.
	Expiry int64 `json:"expiry,omitempty"`
	// The original TTL of this address.
	Ttl int64 `json:"ttl,omitempty"`
}

type AddrBookRecord_CertifiedRecord struct {
	// The Seq counter from the signed PeerRecord envelope
	Seq uint64 `json:"seq,omitempty"`
	// The serialized bytes of the SignedEnvelope containing the PeerRecord. (encoded as hex)
	Raw string `json:"raw,omitempty"`
}

type Eth2Data struct {
	Metadata      *methods.MetaData `json:"metadata,omitempty"`
	MetadataClaim methods.SeqNr     `json:"metadata_claim,omitempty"`
	Status        *methods.Status   `json:"status,omitempty"`
	ENR           *ENRData          `json:"enr,omitempty"`
}

type PartialPeerstoreEntry struct {
	Eth2            *Eth2Data       `json:"eth2,omitempty"`
	AddrRecords     *AddrBookRecord `json:"addr_records,omitempty"`
	Protocols       []string        `json:"protocols,omitempty"`
	ProtocolVersion string          `json:"protocol_version,omitempty"`
	UserAgent       string          `json:"user_agent,omitempty"`
	Pubkey          string          `json:"pub,omitempty"` // raw bytes, not the protobuf representation
	NodeID          string          `json:"node_id,omitempty"`
}

func dsPeerId(v string) peer.ID {
	id, _ := base32.RawStdEncoding.DecodeString(v)
	return peer.ID(id)
}

type Entry struct {
	Key   string
	Value interface{} // should be either something json-encodeable
}

var UnknownKey = errors.New("unknown key")
var IncompleteKey = errors.New("incomplete key")

// Hack to translate peerstore data and paths into nice json updates

func KeyToPath(k ds.Key) (id peer.ID, p string, err error) {
	parts := k.Namespaces()
	if len(parts) == 0 {
		return
	}
	switch parts[0] {
	case "peers":
		if len(parts) < 3 || (parts[1] != "addrs" && len(parts) < 4) {
			err = fmt.Errorf("%w key: %s", IncompleteKey, k)
			return
		}
		id = dsPeerId(parts[2])
		switch parts[1] {
		case "eth2":
			switch parts[3] {
			case "metadata":
				p = "eth2/metadata"
			case "metadata_claim":
				p = "eth2/metadata_claim"
			case "status":
				p = "eth2/status"
			case "enr":
				p = "eth2/enr"
			default:
				err = fmt.Errorf("%w key: %s", UnknownKey, k)
			}
		case "addrs":
			p = "addr_records"
		case "metadata":
			switch parts[3] {
			case "protocols":
				p = "protocols"
			case "ProtocolVersion":
				p = "protocol_version"
			case "AgentVersion":
				p = "user_agent"
			default:
				err = fmt.Errorf("%w key: %s", UnknownKey, k)
			}
		case "keys":
			switch parts[3] {
			case "pub":
				break // never deleted from tee
			case "priv":
				break // ignored.
			default:
				err = fmt.Errorf("%w key: %s", UnknownKey, k)
			}
		}
	default:
		err = fmt.Errorf("%w key: %s", UnknownKey, k)
	}
	return
}

func ItemToEntry(k ds.Key, v []byte) (id peer.ID, out PartialPeerstoreEntry, err error) {
	parts := k.Namespaces()
	if len(parts) == 0 {
		return
	}
	switch parts[0] {
	case "peers":
		if len(parts) < 3 || (parts[1] != "addrs" && len(parts) < 4) {
			err = fmt.Errorf("%w key: %s", IncompleteKey, k)
			return
		}
		id = dsPeerId(parts[2])
		switch parts[1] {
		case "eth2":
			out.Eth2 = &Eth2Data{}
			switch parts[3] {
			case "metadata":
				var md methods.MetaData
				if err := zssz.Decode(bytes.NewReader(v), uint64(len(v)), &md, methods.MetaDataSSZ); err == nil {
					out.Eth2.Metadata = &md
				}
			case "metadata_claim":
				if len(v) == 8 {
					out.Eth2.MetadataClaim = methods.SeqNr(binary.LittleEndian.Uint64(v))
				}
			case "status":
				var st methods.Status
				if err := zssz.Decode(bytes.NewReader(v), uint64(len(v)), &st, methods.StatusSSZ); err == nil {
					out.Eth2.Status = &st
				}
			case "enr":
				out.Eth2.ENR.Raw = string(v)
				rec, err := addrutil.ParseEnr(out.Eth2.ENR.Raw)
				if err == nil {
					enrPairs := rec.AppendElements(nil)
					enrKV := make(map[string]string)
					for i := 1; i < len(enrPairs); i += 2 {
						key := enrPairs[i].(string)
						rawValue := enrPairs[i+1].(rlp.RawValue)

						getTypedValue, ok := addrutil.EnrEntries[key]
						if !ok {
							enrKV["raw_"+key] = hex.EncodeToString(rawValue)
						} else {
							typedValue, getValueStr := getTypedValue()
							if err := rlp.DecodeBytes(rawValue, typedValue); err != nil {
								enrKV["fail_"+key] = hex.EncodeToString(rawValue)
							}
							enrKV[key] = getValueStr()
						}
					}
					out.Eth2.ENR.Contents = enrKV
				}
			default:
				err = fmt.Errorf("%w key: %s", UnknownKey, k)
			}
		case "addrs":
			var addrsPb pstore_pb.AddrBookRecord
			if err = addrsPb.Unmarshal(v); err == nil {
				out.AddrRecords = &AddrBookRecord{}
				for _, a := range addrsPb.Addrs {
					out.AddrRecords.Addrs = append(out.AddrRecords.Addrs, &AddrBookRecord_AddrEntry{
						Addr:   a.Addr.Multiaddr,
						Expiry: a.Expiry,
						Ttl:    a.Ttl,
					})
				}
				if out.AddrRecords.CertifiedRecord != nil {
					out.AddrRecords.CertifiedRecord = &AddrBookRecord_CertifiedRecord{
						Seq: addrsPb.CertifiedRecord.Seq,
						Raw: hex.EncodeToString(addrsPb.CertifiedRecord.Raw),
					}
				}
			}
		case "metadata":
			var res interface{}
			if err = gob.NewDecoder(bytes.NewReader(v)).Decode(&res); err != nil {
				return
			}
			switch parts[3] {
			case "protocols":
				for p := range res.(map[string]struct{}) {
					out.Protocols = append(out.Protocols, p)
				}
			case "ProtocolVersion":
				out.UserAgent = res.(string)
			case "AgentVersion":
				out.UserAgent = res.(string)
			default:
				err = fmt.Errorf("%w key: %s", UnknownKey, k)
			}
		case "keys":
			switch parts[3] {
			case "pub":
				var pub ic.PubKey
				pub, err = ic.UnmarshalPublicKey(v)
				if err == nil {
					ecdsaPub := (*ecdsa.PublicKey)((pub).(*ic.Secp256k1PublicKey))
					b, _ := ((*ic.Secp256k1PublicKey)(ecdsaPub)).Raw()
					out.Pubkey = hex.EncodeToString(b)
					out.NodeID = enode.PubkeyToIDV4(ecdsaPub).String()
				}
			case "priv":
				break // ignored.
			default:
				err = fmt.Errorf("%w key: %s", UnknownKey, k)
			}
		}
	default:
		err = fmt.Errorf("%w key: %s", UnknownKey, k)
	}
	return
}
