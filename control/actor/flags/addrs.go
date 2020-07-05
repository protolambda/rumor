package flags

import (
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/protolambda/rumor/p2p/addrutil"
)

type EnrFlag struct {
	ENR *enr.Record
}

func (f *EnrFlag) String() string {
	if f == nil || f.ENR == nil {
		return "nil ENR"
	}
	enrStr, err := addrutil.EnrToString(f.ENR)
	if err != nil {
		return "? (invalid ENR)"
	}
	return enrStr
}

func (f *EnrFlag) Set(v string) error {
	enrAddr, err := addrutil.ParseEnr(v)
	if err != nil {
		return err
	}
	f.ENR = enrAddr
	return nil
}

func (f *EnrFlag) Type() string {
	return "ENR"
}

type EnodeFlag struct {
	Enode *enode.Node
}

func (f *EnodeFlag) String() string {
	if f == nil || f.Enode == nil {
		return "nil enode"
	}
	return f.Enode.String()
}

func (f *EnodeFlag) Set(v string) error {
	en, err := addrutil.ParseEnode(v)
	if err != nil {
		return err
	}
	f.Enode = en
	return nil
}

func (f *EnodeFlag) Type() string {
	return "enode"
}

type EnrOrEnodeFlag struct {
	Enode *enode.Node
}

func (f *EnrOrEnodeFlag) String() string {
	if f == nil || f.Enode == nil {
		return "nil enode"
	}
	return f.Enode.String()
}

func (f *EnrOrEnodeFlag) Set(v string) error {
	en, err := addrutil.ParseEnrOrEnode(v)
	if err != nil {
		return err
	}
	f.Enode = en
	return nil
}

func (f *EnrOrEnodeFlag) Type() string {
	return "Enr/Enode"
}

type FlexibleAddrFlag struct {
	MultiAddr   ma.Multiaddr
	OptionalEnr *enode.Node
}

func (f *FlexibleAddrFlag) String() string {
	if f == nil || f.MultiAddr == nil {
		return "nil flex addr"
	}
	return f.MultiAddr.String()
}

func (f *FlexibleAddrFlag) Set(v string) error {
	muAddr, err := ma.NewMultiaddr(v)
	if err != nil {
		en, err := addrutil.ParseEnrOrEnode(v)
		if err != nil {
			return err
		}
		f.OptionalEnr = en
		muAddr, err = addrutil.EnodeToMultiAddr(en)
		if err != nil {
			return err
		}
	}

	f.MultiAddr = muAddr
	return nil
}

func (f *FlexibleAddrFlag) Type() string {
	return "ENR/enode/Multi-addr"
}

type NodeIDFlag struct {
	ID enode.ID
}

func (f *NodeIDFlag) String() string {
	if f == nil {
		return "nil node ID"
	}
	return f.ID.String()
}

func (f *NodeIDFlag) Set(v string) error {
	id, err := addrutil.ParseNodeID(v)
	if err != nil {
		return err
	}
	f.ID = id
	return nil
}

func (f *NodeIDFlag) Type() string {
	return "Dv5 node ID"
}

type NodeIDFlexibleFlag struct {
	ID enode.ID
}

func (f *NodeIDFlexibleFlag) String() string {
	if f == nil {
		return "nil node ID"
	}
	return f.ID.String()
}

func (f *NodeIDFlexibleFlag) Set(v string) error {
	id, err := addrutil.ParseNodeIDOrEnrOrEnode(v)
	if err != nil {
		return err
	}
	f.ID = id
	return nil
}

func (f *NodeIDFlexibleFlag) Type() string {
	return "Dv5 flexible ID"
}

type PeerIDFlag struct {
	PeerID peer.ID
}

func (f *PeerIDFlag) String() string {
	if f == nil {
		return "nil peer id"
	}
	return f.PeerID.String()
}

func (f *PeerIDFlag) Set(v string) error {
	id, err := peer.Decode(v)
	if err != nil {
		return err
	}
	f.PeerID = id
	return nil
}

func (f *PeerIDFlag) Type() string {
	return "Peer ID"
}
