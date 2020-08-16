package metadata

import (
	"context"
	"fmt"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/rpc/reqresp"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/sirupsen/logrus"
	"time"
)

type PeerMetadataReqCmd struct {
	*base.Base
	*PeerMetadataState
	Book        track.MetadataBook
	Timeout     time.Duration         `ask:"--timeout" help:"request timeout, 0 to disable"`
	Compression flags.CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
	PeerID      flags.PeerIDFlag      `ask:"<peer-id>" help:"Peer to fetch metadata from."`
}

func (c *PeerMetadataReqCmd) Help() string {
	return "Fetch metadata of connected peer (without pinging first)."
}

func (c *PeerMetadataReqCmd) Default() {
	c.Timeout = 10 * time.Second
	c.Compression = flags.CompressionFlag{Compression: reqresp.SnappyCompression{}}
}

func (c *PeerMetadataReqCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	reqCtx := ctx
	if c.Timeout != 0 {
		reqCtx, _ = context.WithTimeout(reqCtx, c.Timeout)
	}
	code, msg, metadata, err := c.fetch(c.Book, h.NewStream, reqCtx, c.PeerID.PeerID, c.Compression.Compression)
	if err != nil {
		return fmt.Errorf("failed to fetch metadata: %v", err)
	} else {
		if code == reqresp.SuccessCode {
			c.Log.WithField("code", code).WithFields(metadata.Data()).Debug("metadata request success")
		} else {
			c.Log.WithFields(logrus.Fields{
				"code": code,
				"msg":  msg,
			}).Debug("metadata request non-success")
		}
	}
	return nil
}
