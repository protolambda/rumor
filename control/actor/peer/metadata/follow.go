package metadata

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/sirupsen/logrus"
)

type PeerMetadataFollowCmd struct {
	*base.Base
	*PeerMetadataState
	Following bool `ask:"[following]" help:"If the metadata should automatically follow the latest information"`
}

func (c *PeerMetadataFollowCmd) Default() {
	c.Following = true
}

func (c *PeerMetadataFollowCmd) Help() string {
	return "Enable or disable automatic metadata updating"
}

func (c *PeerMetadataFollowCmd) Run(ctx context.Context, args ...string) error {
	c.PeerMetadataState.Following = c.Following

	c.Log.WithFields(logrus.Fields{
		"following": c.PeerMetadataState.Following,
	}).Info("Metadata follow settings")
	return nil
}
