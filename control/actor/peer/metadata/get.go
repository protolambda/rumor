package metadata

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/sirupsen/logrus"
)

type PeerMetadataGetCmd struct {
	*base.Base
	*PeerMetadataState
}

func (c *PeerMetadataGetCmd) Help() string {
	return "Get current metadata and if following automatically or not."
}

func (c *PeerMetadataGetCmd) Run(ctx context.Context, args ...string) error {
	c.Log.WithFields(logrus.Fields{
		"following": c.PeerMetadataState.Following,
		"metadata":  c.PeerMetadataState.Local.Data(),
	}).Info("Metadata settings")
	return nil
}
