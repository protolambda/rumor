package gossip

import (
	"context"
	"errors"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/gossip"
	"sync"
)

type GossipState struct {
	GsNode  gossip.GossipSub
	CloseGS context.CancelFunc
	// string -> *pubsub.Topic
	Topics sync.Map
}

type GossipCmd struct {
	*base.Base
}

func (c *GossipCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "start":
		cmd = &GossipStartCmd{Base: c.Base}
	case "list":
		cmd = &GossipListCmd{Base: c.Base}
	case "join":
		cmd = &GossipJoinCmd{Base: c.Base}
	case "events":
		cmd = &GossipEventsCmd{Base: c.Base}
	case "list-peers":
		cmd = &GossipListPeersCmd{Base: c.Base}
	case "blacklist":
		cmd = &GossipBlacklistCmd{Base: c.Base}
	case "leave":
		cmd = &GossipLeaveCmd{Base: c.Base}
	case "log":
		cmd = &GossipLogCmd{Base: c.Base}
	case "publish":
		cmd = &GossipPublishCmd{Base: c.Base}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *GossipCmd) Routes() []string {
	return []string{"start", "list", "join", "events", "list-peers", "blacklist", "leave", "log", "publish"}
}

func (c *GossipCmd) Help() string {
	return "Manage Libp2p GossipSub"
}

var NoGossipErr = errors.New("Must start gossip-sub first. Try 'gossip start'")


