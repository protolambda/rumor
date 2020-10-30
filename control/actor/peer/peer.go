package peer

import (
	"errors"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/peer/metadata"
	"github.com/protolambda/rumor/control/actor/peer/status"
	trackcmd "github.com/protolambda/rumor/control/actor/peer/track"
	"github.com/protolambda/rumor/p2p/track"
)

type PeerCmd struct {
	*base.Base
	*status.PeerStatusState
	*metadata.PeerMetadataState
	Store track.ExtendedPeerstore
}

func (c *PeerCmd) Cmd(route string) (cmd interface{}, err error) {
	if c.Store == nil {
		return nil, errors.New("Not available. Create a peerstore first.")
	}
	switch route {
	case "connect":
		cmd = &PeerConnectCmd{Base: c.Base, Store: c.Store}
	case "disconnect":
		cmd = &PeerDisconnectCmd{Base: c.Base}
	case "connectall":
		cmd = &PeerConnectAllCmd{Base: c.Base, Store: c.Store}
	case "protect":
		cmd = &PeerProtectCmd{Base: c.Base}
	case "unprotect":
		cmd = &PeerUnprotectCmd{Base: c.Base}
	case "add":
		cmd = &PeerAddCmd{Base: c.Base, Store: c.Store}
	case "trim":
		cmd = &PeerTrimCmd{Base: c.Base}
	case "list":
		cmd = &PeerListCmd{Base: c.Base, Store: c.Store}
	case "info":
		cmd = &PeerInfoCmd{Base: c.Base, Store: c.Store}
	case "identify":
		cmd = &PeerIdentifyCmd{Base: c.Base}
	case "track":
		cmd = &trackcmd.PeerTrack{Base: c.Base, Store: c.Store}
	case "addrs":
		cmd = &PeerAddrsCmd{Base: c.Base}
	case "status":
		cmd = &status.PeerStatusCmd{Base: c.Base, PeerStatusState: c.PeerStatusState, Book: c.Store}
	case "metadata":
		cmd = &metadata.PeerMetadataCmd{Base: c.Base, PeerMetadataState: c.PeerMetadataState, Store: c.Store}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *PeerCmd) Routes() []string {
	return []string{"connect", "disconnect", "connectall", "protect", "unprotect", "add", "trim",
		"list", "info", "identify", "track", "addrs", "status", "metadata"}
}

func (c *PeerCmd) Help() string {
	return "Manage peers"
}
