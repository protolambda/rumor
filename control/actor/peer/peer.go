package peer

import (
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
)

type PeerCmd struct {
	*base.Base
}

func (c *PeerCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "connect":
		cmd = &PeerConnectCmd{Base: c.Base}
	case "disconnect":
		cmd = &PeerDisconnectCmd{Base: c.Base}
	case "protect":
		cmd = &PeerProtectCmd{Base: c.Base}
	case "unprotect":
		cmd = &PeerUnprotectCmd{Base: c.Base}
	case "trim":
		cmd = &PeerTrimCmd{Base: c.Base}
	case "list":
		cmd = &PeerListCmd{Base: c.Base}
	case "addrs":
		cmd = &PeerAddrsCmd{Base: c.Base}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *PeerCmd) Routes() []string {
	return []string{"start"}
}

func (c *PeerCmd) Help() string {
	return "Manage the libp2p peerstore"
}
