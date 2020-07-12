package dv5

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/rumor/p2p/peering/dv5"
	"net"
)

type Dv5RunCmd struct {
	*base.Base
	*Dv5State
	dv5.Dv5Settings
	ListenIP  net.IP   `ask:"--ip" help:"Optional listen IP. Will try 0.0.0.0 otherwise."`
	ListenUDP uint16   `ask:"--udp" help:"Optional UDP port. Will try ENR port otherwise."`
	Bootnodes []string `ask:"[bootnodes]" help:"Bootnodes for dv5"`
}

func (c *Dv5RunCmd) Help() string {
	return "Run discv5, spawned as a background process."
}

func (c *Dv5RunCmd) Run(ctx context.Context, args ...string) error {
	if c.Dv5State.Dv5Node != nil {
		return fmt.Errorf("Already have dv5 open at %s", c.Dv5State.Dv5Node.Self().String())
	}
	bootNodes := make([]*enode.Node, 0, len(c.Bootnodes))
	for i := 0; i < len(c.Bootnodes); i++ {
		dv5Addr, err := addrutil.ParseEnrOrEnode(c.Bootnodes[i])
		if err != nil {
			return fmt.Errorf("bootnode %d is bad: %v", i, err)
		}
		bootNodes = append(bootNodes, dv5Addr)
	}
	ip := c.ListenIP
	if ip == nil {
		ip = net.IPv4zero
	}
	udpPort := c.ListenUDP
	if udpPort == 0 {
		localNode := c.LocalNode()
		udpPort = uint16(localNode.Node().UDP())
	}
	var err error
	c.Dv5State.Dv5Node, err = dv5.NewDiscV5(c.Log, ip, udpPort, c.Dv5Settings, bootNodes)
	if err != nil {
		return fmt.Errorf("failed to init new dv5: %v", err)
	}
	c.Log.Infof("Started discv5 with %d bootnodes", len(bootNodes))

	c.Control.RegisterStop(func(ctx context.Context) error {
		c.Dv5State.Dv5Node.Close()
		c.Dv5State.Dv5Node = nil
		log.Info("Stopped discv5")
		return nil
	})
	return nil
}
