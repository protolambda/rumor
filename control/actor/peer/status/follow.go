package status

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/sirupsen/logrus"
)

type PeerStatusFollowCmd struct {
	*base.Base
	*PeerStatusState
	Following bool `ask:"[following]" help:"If the status should automatically follow the current chain (if any)"`
}

func (c *PeerStatusFollowCmd) Default() {
	c.Following = true
}

func (c *PeerStatusFollowCmd) Help() string {
	return "Enable or disable automatic status updating based on followed chain"
}

func (c *PeerStatusFollowCmd) Run(ctx context.Context, args ...string) error {
	c.PeerStatusState.Following = c.Following

	c.Log.WithFields(logrus.Fields{
		"following": c.PeerStatusState.Following,
	}).Info("Status follow settings")
	return nil
}
