package dv5

import (
	"context"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/rumor/p2p/peering/dv5"
)

type Dv5StartCmd struct {
	*base.Base
}

func (c *Dv5StartCmd) Help() string {
	return "Start discv5."
}

func (c *Dv5StartCmd) Run(ctx context.Context, args ...string) error {
	_, err := c.Host()
	if err != nil {
		return err
	}
	if c.IP == nil {
		return errors.New("Host has no IP yet. Get with 'host listen'")
	}
	if c.Dv5State.Dv5Node != nil {
		return fmt.Errorf("Already have dv5 open at %s", c.Dv5State.Dv5Node.Self().String())
	}
	bootNodes := make([]*enode.Node, 0, len(args))
	for i := 1; i < len(args); i++ {
		dv5Addr, err := addrutil.ParseEnrOrEnode(args[i])
		if err != nil {
			return err
		}
		bootNodes = append(bootNodes, dv5Addr)
	}
	c.Dv5State.Dv5Node, err = dv5.NewDiscV5(c.Log, c.IP, c.UdpPort, c.PrivKey, bootNodes)
	if err != nil {
		return err
	}
	log.Info("Started discv5")
	return nil
}
