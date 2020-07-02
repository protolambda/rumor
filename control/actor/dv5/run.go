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

type Dv5RunCmd struct {
	*base.Base
	*Dv5State
	WithPriv
	Bootnodes []string `ask:"[bootnodes]" help:"Bootnodes for dv5"`
}

func (c *Dv5RunCmd) Help() string {
	return "Run discv5, spawned as a background process."
}

func (c *Dv5RunCmd) Run(ctx context.Context, args ...string) error {
	_, err := c.Host()
	if err != nil {
		return err
	}
	ip := c.GetIP()
	udpPort := c.GetUDP()
	priv := c.GetPriv()

	if ip == nil {
		return errors.New("Host has no IP yet. Get with 'host listen'")
	}
	if c.Dv5State.Dv5Node != nil {
		return fmt.Errorf("Already have dv5 open at %s", c.Dv5State.Dv5Node.Self().String())
	}
	bootNodes := make([]*enode.Node, 0, len(c.Bootnodes))
	for i := 0; i < len(c.Bootnodes); i++ {
		dv5Addr, err := addrutil.ParseEnrOrEnode(c.Bootnodes[i])
		if err != nil {
			return err
		}
		bootNodes = append(bootNodes, dv5Addr)
	}
	c.Dv5State.Dv5Node, err = dv5.NewDiscV5(c.Log, ip, udpPort, priv, bootNodes)
	if err != nil {
		return err
	}
	c.Log.Infof("Started discv5 with %d bootnodes", len(bootNodes))

	spCtx, freed := c.SpawnContext()
	go func() {
		<-spCtx.Done()

		c.Dv5State.Dv5Node.Close()
		c.Dv5State.Dv5Node = nil
		log.Info("Stopped discv5")

		freed()
	}()
	return nil
}
