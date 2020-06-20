package status

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/sirupsen/logrus"
)

type PeerStatusGetCmd struct {
	*base.Base
	*PeerStatusState
}

func (c *PeerStatusGetCmd) Help() string {
	return "Get current status and if following the chain or not."
}

func (c *PeerStatusGetCmd) Run(ctx context.Context, args ...string) error {
	c.Log.WithFields(logrus.Fields{
		"following": c.PeerStatusState.Following,
		"status":    c.PeerStatusState.Local.Data(),
	}).Info("Status settings")
	return nil
}
