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
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/ztyp/codec"
	"net"
	"sort"
	"strconv"
)

/*
Peerstore layout, partially due to libp2p legacy:

	/peers
		- /eth2
		 	- /<peer-id>
				- /metadata           <- ssz encoded
				- /metadata_claim     <- ssz encoded
				- /status             <- ssz encoded
				- /enr                <- stored in raw base64 enr presentation. Then expanded into subfields when reading:
				  - /raw              <- base64 enr representation
                  - /other            <- map of unrecognized key/value pairs. Values encoded as hex bytes by us.
                  - /eth2_data        <- Eth2Data
                  - /attnets          <- bitfield
                  - /seq              <- ENR seq num
                  - /ip               <- IP (v4 or v6)
                  - /udp              <- UDP port
                  - /tcp              <- TCP port

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
	Raw      string             `json:"raw,omitempty"`
	Other    map[string]string  `json:"other,omitempty"`
	Eth2Data *beacon.Eth2Data   `json:"eth2_data,omitempty"`
	Attnets  *beacon.AttnetBits `json:"attnets,omitempty"`
	Seq      uint64             `json:"seq,omitempty"`
	IP       net.IP             `json:"ip,omitempty"`
	TCP      uint16             `json:"tcp,omitempty"`
	UDP      uint16             `json:"udp,omitempty"`
}

type AddrBookRecord struct {
	// The multiaddresses. This is a sorted list where element 0 expires the soonest.
	Addrs []*AddrBookRecord_AddrEntry `json:"addrs,omitempty"`
	// The most recently received signed PeerRecord.
	CertifiedRecord *AddrBookRecord_CertifiedRecord `json:"certified_record,omitempty"`
}

type AddrBookRecord_AddrEntry struct {
	// Addr in multi-addr format
	Addr string `json:"addr,omitempty"`
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
	Metadata      *beacon.MetaData `json:"metadata,omitempty"`
	MetadataClaim beacon.SeqNr     `json:"metadata_claim,omitempty"`
	Status        *beacon.Status   `json:"status,omitempty"`
	ENR           *ENRData         `json:"enr,omitempty"`
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

func (p *PartialPeerstoreEntry) Merge(other *PartialPeerstoreEntry) {
	if other.Eth2 != nil {
		if p.Eth2 == nil {
			p.Eth2 = other.Eth2
		} else {
			if other.Eth2.ENR != nil {
				// only ever update ENR forwards
				if p.Eth2.ENR == nil || other.Eth2.ENR.Seq > p.Eth2.ENR.Seq {
					p.Eth2.ENR = other.Eth2.ENR
				}
			}
			if other.Eth2.Status != nil {
				p.Eth2.Status = other.Eth2.Status
			}
			if other.Eth2.MetadataClaim > p.Eth2.MetadataClaim {
				p.Eth2.MetadataClaim = other.Eth2.MetadataClaim
			}
			if other.Eth2.Metadata != nil {
				// only ever update metadata forwards
				if p.Eth2.Metadata == nil || other.Eth2.Metadata.SeqNumber > p.Eth2.Metadata.SeqNumber {
					p.Eth2.Metadata = other.Eth2.Metadata
				}
			}
		}
	}
	if other.AddrRecords != nil {
		p.AddrRecords = other.AddrRecords
	}
	if other.Protocols != nil {
		p.Protocols = other.Protocols
	}
	if other.ProtocolVersion != "" {
		p.ProtocolVersion = other.ProtocolVersion
	}
	if other.UserAgent != "" {
		p.UserAgent = other.UserAgent
	}
	if other.Pubkey != "" {
		p.Pubkey = other.Pubkey
	}
	if other.NodeID != "" {
		p.NodeID = other.NodeID
	}
}

func (p *PartialPeerstoreEntry) ToCSV(prefixFields ...string) (out [][]string) {
	entry := func(key string, value string) {
		fields := make([]string, 0, len(prefixFields)+2)
		fields = append(fields, prefixFields...)
		fields = append(fields, key, value)
		out = append(out, fields)
	}
	if p.Eth2 != nil {
		if p.Eth2.ENR != nil {
			entry("eth2/enr", p.Eth2.ENR.Raw)
			if p.Eth2.ENR.Eth2Data != nil { // combine these fields, since they are always updated together
				eth2CSVStr := fmt.Sprintf("%s:%s:%d",
					p.Eth2.ENR.Eth2Data.ForkDigest,
					p.Eth2.ENR.Eth2Data.NextForkVersion,
					p.Eth2.ENR.Eth2Data.NextForkEpoch)
				entry("eth2/enr/eth2_data", eth2CSVStr)
			}
			if p.Eth2.ENR.Attnets != nil {
				entry("eth2/enr/attnets", p.Eth2.ENR.Attnets.String())
			}
			for k, v := range p.Eth2.ENR.Other {
				entry("eth2/enr/"+k, v)
			}
			if p.Eth2.ENR.Seq != 0 {
				entry("eth2/enr/seq", strconv.FormatUint(p.Eth2.ENR.Seq, 10))
			}
		}
		if p.Eth2.Status != nil {
			entry("eth2/status/finalized_root", p.Eth2.Status.FinalizedRoot.String())
			entry("eth2/status/finalized_epoch", strconv.FormatUint(uint64(p.Eth2.Status.FinalizedEpoch), 10))
			entry("eth2/status/head_root", p.Eth2.Status.HeadRoot.String())
			entry("eth2/status/head_slot", strconv.FormatUint(uint64(p.Eth2.Status.HeadSlot), 10))
			entry("eth2/status/fork_digest", p.Eth2.Status.ForkDigest.String())
		}
		if p.Eth2.MetadataClaim > p.Eth2.MetadataClaim {
			entry("eth2/metadata_claim", strconv.FormatUint(uint64(p.Eth2.MetadataClaim), 10))
		}
		if p.Eth2.Metadata != nil {
			entry("eth2/metadata/seq_number", strconv.FormatUint(uint64(p.Eth2.Metadata.SeqNumber), 10))
			entry("eth2/metadata/attnets", p.Eth2.Metadata.Attnets.String())
		}
	}
	if p.AddrRecords != nil {
		if p.AddrRecords.CertifiedRecord != nil {
			entry("addr_records/certified_record", p.AddrRecords.CertifiedRecord.Raw)
		}
		if p.AddrRecords.Addrs != nil {
			for i, addr := range p.AddrRecords.Addrs {
				if addr.Addr != "" {
					entry(fmt.Sprintf("addr_records/addrs/%d/addr", i), addr.Addr)
				}
				if addr.Ttl != 0 {
					entry(fmt.Sprintf("addr_records/addrs/%d/ttl", i), fmt.Sprintf("%d", addr.Ttl))
				}
				if addr.Expiry != 0 {
					entry(fmt.Sprintf("addr_records/addrs/%d/expiry", i), fmt.Sprintf("%d", addr.Expiry))
				}
			}
			entry("addr_records/addrs/count", fmt.Sprintf("%d", len(p.AddrRecords.Addrs)))
		}
	}
	if p.Protocols != nil {
		for i, prot := range p.Protocols {
			entry(fmt.Sprintf("protocols/%d", i), prot)
		}
		entry("protocols/count", fmt.Sprintf("%d", len(p.Protocols)))
	}
	if p.ProtocolVersion != "" {
		entry("protocol_version", p.ProtocolVersion)
	}
	if p.UserAgent != "" {
		entry("user_agent", p.UserAgent)
	}
	if p.Pubkey != "" {
		entry("pub", p.Pubkey)
	}
	if p.NodeID != "" {
		entry("node_id", p.NodeID)
	}
	return
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
				var md beacon.MetaData
				if e := md.Deserialize(codec.NewDecodingReader(bytes.NewReader(v), uint64(len(v)))); e == nil {
					out.Eth2.Metadata = &md
				} else {
					err = fmt.Errorf("bad metadata in peerstore: %v", e)
					return
				}
			case "metadata_claim":
				if len(v) == 8 {
					out.Eth2.MetadataClaim = beacon.SeqNr(binary.LittleEndian.Uint64(v))
				} else {
					err = fmt.Errorf("bad metadata_claim in peerstore, wrong length: claim bytes: %x", v)
					return
				}
			case "status":
				var st beacon.Status
				if e := st.Deserialize(codec.NewDecodingReader(bytes.NewReader(v), uint64(len(v)))); e == nil {
					out.Eth2.Status = &st
				} else {
					err = fmt.Errorf("bad status in peerstore: %v", e)
					return
				}
			case "enr":
				out.Eth2.ENR = &ENRData{}
				out.Eth2.ENR.Raw = string(v)
				rec, err := addrutil.ParseEnr(out.Eth2.ENR.Raw)
				if err == nil {
					n, err := enode.New(enode.ValidSchemes, rec)
					if err == nil {
						if eth2Data, ok, err := addrutil.ParseEnrEth2Data(n); err != nil && ok {
							out.Eth2.ENR.Eth2Data = eth2Data
						}
						if attnets, ok, err := addrutil.ParseEnrAttnets(n); err != nil && ok {
							out.Eth2.ENR.Attnets = attnets
						}
						out.Eth2.ENR.Seq = n.Seq()
						out.Eth2.ENR.IP = n.IP()
						out.Eth2.ENR.TCP = uint16(n.TCP())
						out.Eth2.ENR.UDP = uint16(n.UDP())
					}
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
							// if these cannot be parsed, then fine, add the raw form on failure (see above)
							// Otherwise, don't duplicat the data.
							if key == "eth2" || key == "attnets" || key == "ip" ||
								key == "ip6" || key == "udp" || key == "tcp" {
								continue
							}
							enrKV[key] = getValueStr()
						}
					}
					out.Eth2.ENR.Other = enrKV
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
						Addr:   a.Addr.Multiaddr.String(),
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
				sort.Strings(out.Protocols)
			case "ProtocolVersion":
				out.ProtocolVersion = res.(string)
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
