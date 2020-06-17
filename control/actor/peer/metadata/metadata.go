package metadata

import (
	"errors"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/rpc/methods"
)

type PeerMetadataState struct {
	Following bool
	Local     methods.MetaData
}

type PeerMetadataCmd struct {
	*base.Base
}

func (c *PeerMetadataCmd) Help() string {
	return "Manage and track peer metadata"
}

func (c *PeerMetadataCmd) Cmd(route string) (cmd interface{}, err error) {
	/* TODO
		  ping <peer id>  --update # request peer for pong, update metadata maybe
		  pong --update   # serve others with pongs, and if ping is new enough, request them for metadata if --update=true

		  fetch <peer id>  # get metadata of peer

		  poll <interval>  # poll connected peers for metadata by pinging them on interval

		  get
		  set --follow

		  serve   # serve meta data
	actors
	*/
	return nil, errors.New("not implemented yet")
}

