package peerstore

import (
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/track"
)

type PeerstoreCmd struct {
	*base.Base

	GlobalPeerstores track.Peerstores
	CurrentPeerstore track.DynamicPeerstore
}

// TODO: import/export peerstore command

func (c *PeerstoreCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "create":
		cmd = &CreateCmd{Base: c.Base, GlobalPeerstores: c.GlobalPeerstores, CurrentPeerstore: c.CurrentPeerstore}
	case "switch":
		cmd = &SwitchCmd{Base: c.Base, GlobalPeerstores: c.GlobalPeerstores, CurrentPeerstore: c.CurrentPeerstore}
	case "list":
		cmd = &ListCmd{Base: c.Base, GlobalPeerstores: c.GlobalPeerstores, CurrentPeerstore: c.CurrentPeerstore}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *PeerstoreCmd) Routes() []string {
	return []string{"create", "switch", "list"}
}

func (c *PeerstoreCmd) Help() string {
	return "Manage peerstores"
}
