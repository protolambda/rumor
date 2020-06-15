package peer

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/sirupsen/logrus"
	"strings"
	"sync"
	"time"
)

type PeerCmd struct {
	*base.Base
	log    logrus.FieldLogger
}

func (c *PeerCmd) Help() string {
	return "Manage the libp2p peerstore"
}

func (c *PeerCmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "start":
		cmd = DefaultHostStartCmd(c.Actor, c.Log)
	// TODO
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *PeerCmd) PeerList() *PeerListCmd {
	return &PeerListCmd{
		PeerCmd:       c,
		Which:         "connected",
		ListLatency:   false,
		ListProtocols: false,
		ListAddrs:     true,
		ListStatus:    false,
		ListMetadata:  false,
		ListClaimSeq:  false,
	}
}

func (c *PeerCmd) Trim() *PeerTrimCmd {
	return &PeerTrimCmd{
		PeerCmd: c,
		Timeout: time.Second * 2,
	}
}



func parseRoot(v string) ([32]byte, error) {
	if v == "0" {
		return [32]byte{}, nil
	}
	if strings.HasPrefix(v, "0x") {
		v = v[2:]
	}
	if len(v) != 64 {
		return [32]byte{}, fmt.Errorf("provided root has length %d, expected 64 hex characters (ignoring optional 0x prefix)", len(v))
	}
	var out [32]byte
	_, err := hex.Decode(out[:], []byte(v))
	return out, err
}

func parseForkVersion(v string) ([4]byte, error) {
	if strings.HasPrefix(v, "0x") {
		v = v[2:]
	}
	if len(v) != 8 {
		return [4]byte{}, fmt.Errorf("provided fork version has length %d, expected 8 hex characters (ignoring optional 0x prefix)", len(v))
	}
	var out [4]byte
	_, err := hex.Decode(out[:], []byte(v))
	return out, err
}

type PeerMetadataCmd struct {
	*PeerCmd
}

func (c *PeerMetadataCmd) Help() string {
	return "Manage and track peer metadata"
}

func (c *PeerMetadataCmd) Get(ctx context.Context, args ...string) (cmd interface{}, remaining []string, err error) {
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
	return nil, nil, errors.New("not implemented yet")
}
