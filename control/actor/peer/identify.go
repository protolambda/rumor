package peer

import (
	"context"
	"errors"
	"github.com/libp2p/go-libp2p/p2p/protocol/identify"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"time"
)

type PeerIdentifyCmd struct {
	*base.Base
	Timeout time.Duration    `ask:"--timeout" help:"connection timeout, 0 to disable"`
	PeerID  flags.PeerIDFlag `ask:"<peer-ID>" help:"peer ID"`
}

func (c *PeerIdentifyCmd) Default() {
	c.Timeout = 10 * time.Second
}

func (c *PeerIdentifyCmd) Help() string {
	return "Identify peer (requires host start --identify=true)."
}

type HostWithIDService interface {
	IDService() *identify.IDService
}

func (c *PeerIdentifyCmd) Run(ctx context.Context, args ...string) error {
	h, err := c.Host()
	if err != nil {
		return err
	}
	withIdentify, ok := h.(HostWithIDService)
	if !ok {
		return errors.New("host does not support libp2p identify protocol")
	}
	idService := withIdentify.IDService()
	if idService == nil {
		return errors.New("libp2p identify not enabled on this host")
	}
	var timeout <-chan struct{} = nil
	if c.Timeout != 0 {
		timeoutCtx, _ := context.WithTimeout(ctx, c.Timeout)
		timeout = timeoutCtx.Done()
	}
	if conns := h.Network().ConnsToPeer(c.PeerID.PeerID); len(conns) > 0 {
		select {
		case <-idService.IdentifyWait(conns[0]):
			c.Log.Info("completed identification")
		case <-ctx.Done():
			c.Log.Info("canceled waiting for identification")
		//noinspection ALL
		case <-timeout: // nil if disabled timeout.
			c.Log.Info("awaiting identification timed out")
		}
	} else {
		return errors.New("not connected to peer, cannot await connection identify")
	}
	return nil
}
