package status

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/sirupsen/logrus"
)

type PeerStatusSetCmd struct {
	*base.Base
	*PeerStatusState
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
	st := c.PeerStatusState.Local
	st.HeadForkVersion = c.ForkVersion
	st.HeadRoot = c.HeadRoot
	st.HeadSlot = c.HeadSlot
	st.FinalizedEpoch = c.FinalizedEpoch
	st.FinalizedRoot = c.FinalizedRoot

	c.PeerStatusState.Local = st
	c.PeerStatusState.Following = c.Following

	c.Log.WithFields(logrus.Fields{
		"following": c.PeerStatusState.Following,
		"status":    c.PeerStatusState.Local,
	}).Info("Status settings")
	return nil
}
