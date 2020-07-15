package chcmd

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/zrnt/eth2/beacon"
)

type AttestationCmd struct {
	*base.Base
	Root beacon.Root `ask:"<root>" help:"Root of the attestation (incl bitfield) to add to the chain"`
}

func (c *AttestationCmd) Help() string {
	return "Add an attestation from the attestations tracker to the chain view."
}

func (c *AttestationCmd) Run(ctx context.Context, args ...string) error {
	// TODO
	return nil
}
