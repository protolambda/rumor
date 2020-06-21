package enr

import (
	"context"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/addrutil"
	"net"
)

type EnrMakeCmd struct {
	*base.Base
	IP   net.IP               `ask:"--ip" help:"IP address (v4 or v6). None if empty."`
	TCP  uint16               `ask:"--tcp" help:"TCP port. None if 0"`
	UDP  uint16               `ask:"--udp" help:"UDP port. None if 0"`
	Priv flags.P2pPrivKeyFlag `ask:"--priv" help:"Private key, in raw hex encoded format (32 bytes -> 64 hex chars with optional 0x prefix). None if empty, and also no signed ENR."`
}

func (c *EnrMakeCmd) Help() string {
	return "Make a private key for an ENR."
}

func (c *EnrMakeCmd) Run(ctx context.Context, args ...string) error {
	rec := addrutil.MakeENR(c.IP, c.TCP, c.UDP, c.Priv.Priv)
	enrStr, err := addrutil.EnrToString(rec)
	if err != nil {
		return err
	}
	c.Log.WithField("enr", enrStr).Infof("ENR created!")
	return nil
}
