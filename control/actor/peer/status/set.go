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
	ForkDigest     beacon.ForkDigest `ask:"--fork-digest" help:"Fork digest"`
	HeadRoot       beacon.Root       `ask:"--head-root" help:"Head root"`
	HeadSlot       beacon.Slot       `ask:"--head-slot" help:"Head slot"`
	FinalizedRoot  beacon.Root       `ask:"--finalized-root" help:"Finalized root"`
	FinalizedEpoch beacon.Epoch      `ask:"--finalized-epoch" help:"Finalized epoch"`
	Merge          bool              `ask:"--merge" help:"If true, only apply non-zero options to state"`
}

func (c *PeerStatusSetCmd) Default() {
	c.Merge = true
}

func (c *PeerStatusSetCmd) Help() string {
	return "Set (a part of) the current status and if following the chain or not."
}

func (c *PeerStatusSetCmd) Run(ctx context.Context, args ...string) error {
	st := beacon.Status{}
	if c.Merge {
		st = c.PeerStatusState.Local
	}
	if !c.Merge || c.ForkDigest != (beacon.ForkDigest{}) {
		st.ForkDigest = c.ForkDigest
	}
	if !c.Merge || c.HeadRoot != (beacon.Root{}) {
		st.HeadRoot = c.HeadRoot
	}
	if !c.Merge || c.HeadSlot != 0 {
		st.HeadSlot = c.HeadSlot
	}
	if !c.Merge || c.FinalizedEpoch != 0 {
		st.FinalizedEpoch = c.FinalizedEpoch
	}
	if !c.Merge || c.FinalizedRoot != (beacon.Root{}) {
		st.FinalizedRoot = c.FinalizedRoot
	}
	// mutate full status at once (TODO: do we need a lock?)
	c.PeerStatusState.Local = st

	c.Log.WithFields(logrus.Fields{
		"following": c.PeerStatusState.Following,
		"status":    c.PeerStatusState.Local.Data(),
	}).Info("Status settings")
	return nil
}
