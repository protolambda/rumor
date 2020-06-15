package status

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/sirupsen/logrus"
)

type PeerStatusSetCmd struct {
	*base.Base
	ForkVersion    beacon.Version `ask:"--fork-version"`
	HeadRoot       beacon.Root    `ask:"--head-root"`
	HeadSlot       beacon.Slot    `ask:"--head-slot"`
	FinalizedRoot  beacon.Root    `ask:"--finalized-root"`
	FinalizedEpoch beacon.Epoch   `ask:"--finalized-epoch"`
	Following      bool           `ask:"--following" help:"If the status should automatically follow the current chain (if any)"`
}

func (c *PeerStatusSetCmd) Help() string {
	return "Set (a part of) the current status and if following the chain or not."
}

func (c *PeerStatusSetCmd) Run(ctx context.Context, args ...string) error {
	// TODO: only change each of these if they were modified
	c.PeerStatusState.Local.HeadForkVersion = c.ForkVersion
	c.PeerStatusState.Local.HeadRoot = c.HeadRoot
	c.PeerStatusState.Local.HeadSlot = c.HeadSlot
	c.PeerStatusState.Local.FinalizedEpoch = c.FinalizedEpoch
	c.PeerStatusState.Local.FinalizedRoot = c.FinalizedRoot
	c.PeerStatusState.Following = c.Following

	c.Log.WithFields(logrus.Fields{
		"following": c.PeerStatusState.Following,
		"status":    c.PeerStatusState.Local,
	}).Info("Status settings")
	return nil
}
