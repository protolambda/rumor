package metadata

import (
	"errors"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/rpc/methods"
)

type PeerMetadataState struct {
	Following bool
	Local     methods.MetaData
}

type PeerMetadataCmd struct {
	*base.Base
	*PeerMetadataState
}

func (c *PeerMetadataCmd) Help() string {
	return "Manage and track peer metadata"
}

	/* TODO
		  ping <peer id>  --update # request peer for pong, update metadata maybe
		  pong --update   # serve others with pongs, and if ping is new enough, request them for metadata if --update=true

		  fetch <peer id>  # get metadata of peer

		  poll <interval>  # poll connected peers for metadata by pinging them on interval

		  get
		  set
		  follow

		  serve   # serve meta data
	*/

func (c *PeerMetadataCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "ping":
		cmd = &PeerMetadataPingCmd{Base: c.Base, PeerMetadataState: c.PeerMetadataState}
	case "pong":
		cmd = &PeerMetadataPongCmd{Base: c.Base, PeerMetadataState: c.PeerMetadataState}
	case "get":
		cmd = &PeerMetadataGetCmd{Base: c.Base, PeerMetadataState: c.PeerMetadataState}
	case "set":
		cmd = &PeerMetadataSetCmd{Base: c.Base, PeerMetadataState: c.PeerMetadataState}
	case "req":
		cmd = &PeerMetadataReqCmd{Base: c.Base, PeerMetadataState: c.PeerMetadataState}
	case "poll":
		cmd = &PeerMetadataPollCmd{Base: c.Base, PeerMetadataState: c.PeerMetadataState}
	case "serve":
		cmd = &PeerMetadataServeCmd{Base: c.Base, PeerMetadataState: c.PeerMetadataState}
	case "follow":
		cmd = &PeerMetadataFollowCmd{Base: c.Base, PeerMetadataState: c.PeerMetadataState}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *PeerMetadataCmd) Routes() []string {
	return []string{"ping", "pong", "get", "set", "req", "poll", "serve", "follow"}
}
