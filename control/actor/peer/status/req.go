package status

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

type PeerStatusReqCmd struct {
	*base.Base
	*PeerStatusState
	Book        track.StatusBook
	Timeout     time.Duration         `ask:"--timeout" help:"request timeout, 0 to disable"`
	Compression flags.CompressionFlag `ask:"--compression" help:"Compression. 'none' to disable, 'snappy' for streaming-snappy"`
	PeerID      flags.PeerIDFlag      `ask:"<peer-id>" help:"Peer to fetch status from."`
}

func (c *PeerStatusReqCmd) Help() string {
	return "Fetch status of connected peer."
}

func (c *PeerStatusReqCmd) Default() {
	c.Timeout = 10 * time.Second
	c.Compression = flags.CompressionFlag{Compression: reqresp.SnappyCompression{}}
}

func (c *PeerStatusReqCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	reqCtx := ctx
	if c.Timeout != 0 {
		reqCtx, _ = context.WithTimeout(reqCtx, c.Timeout)
	}
	code, msg, stat, err := c.fetch(c.Book, h.NewStream, reqCtx, c.PeerID.PeerID, c.Compression.Compression)
	if err != nil {
		return fmt.Errorf("failed to fetch status: %v", err)
	} else {
		if code == reqresp.SuccessCode {
			c.Log.WithField("code", code).WithFields(stat.Data()).Debug("status request success")
		} else {
			c.Log.WithFields(logrus.Fields{
				"code": code,
				"msg":  msg,
			}).Debug("status request non-success")
		}
	}
	return nil
}
