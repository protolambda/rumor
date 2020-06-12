package actor

import (
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/protolambda/rumor/p2p/addrutil"
)

type EnrFlag struct {
	ENR *enr.Record
}

func (f *EnrFlag) String() string {
	if f == nil {
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

type EnodeFlag struct {
	Enode *enode.Node
}

func (f *EnodeFlag) String() string {
	if f == nil {
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
	if f == nil {
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
