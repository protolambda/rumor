package track

import (
	"errors"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/track"
)

type PeerTrack struct {
	*base.Base
	Store track.ExtendedPeerstore
}

func (c *PeerTrack) Cmd(route string) (cmd interface{}, err error) {
	if c.Store == nil {
		return nil, errors.New("Not available. Create a peerstore first.")
	}
	switch route {
	case "tee":
		cmd = &PeerTrackTeeCmd{Base: c.Base, Store: c.Store}
	case "list", "ls":
		cmd = &PeerTrackListCmd{Base: c.Base, Store: c.Store}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *PeerTrack) Routes() []string {
	return []string{"tee", "list"}
}

func (c *PeerTrack) Help() string {
	return "Manage peerstore trackers"
}
