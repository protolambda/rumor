package status

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"time"
)

type PeerStatusReqCmd struct {
	*base.Base
	Timeout        time.Duration   `ask:"--timeout" help:"request timeout, 0 to disable"`
	Compression    flags.CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
	PeerID         flags.PeerIDFlag      `ask:"<peer-id>" help:"Peer to fetch status from."`
}

func (c *PeerStatusReqCmd) Help() string {
	return "Fetch status of connected peer."
}

func (c *PeerStatusReqCmd) Run(ctx context.Context, args ...string) error {
	return c.fetch(ctx, c.Timeout, c.PeerID.PeerID, c.Compression.Compression)
}
