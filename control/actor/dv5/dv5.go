package dv5

import (
	"context"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/protolambda/ask"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/rumor/p2p/peering/dv5"
)

type Dv5Cmd struct {
	*RootCmd
	*Dv5State `ask:"-"`
}

func (c *Dv5Cmd) Cmd(route string) (cmd interface{}, err error) {
	switch route {
	case "start":
		cmd = &Dv5StartCmd{Dv5Cmd: c}
	case "stop":
		cmd = &Dv5StopCmd{Dv5Cmd: c}
	case "ping":
		cmd = &Dv5PingCmd{Dv5Cmd: c}
	case "resolve":
		cmd = &Dv5ResolveCmd{Dv5Cmd: c}
	case "request":
		cmd = &Dv5RequestCmd{Dv5Cmd: c}
	case "lookup":
		cmd = &Dv5LookupCmd{Dv5Cmd: c}
	case "random":
		cmd = &Dv5LookupRandomCmd{Dv5Cmd: c}
	case "self":
		cmd = &Dv5SelfCmd{Dv5Cmd: c}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *Dv5Cmd) Routes() []string {
	return []string{"start", "stop", "ping", "resolve", "request", "lookup", "random", "self"}
}

func (c *Dv5Cmd) Help() string {
	return "Peer discovery with discv5"
}

type Dv5StartCmd struct {
	*Dv5Cmd
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

var NoDv5Err = errors.New("Must start discv5 first. Try 'dv5 start'")

type Dv5StopCmd struct {
	*Dv5Cmd
}

func (c *Dv5StopCmd) Help() string {
	return "Stop discv5"
}

func (c *Dv5StopCmd) Run(ctx context.Context, args ...string) error {
	if c.Dv5State.Dv5Node == nil {
		return NoDv5Err
	}
	c.Dv5State.Dv5Node.Close()
	c.Dv5State.Dv5Node = nil
	log.Info("Stopped discv5")
	return nil
}

type Dv5PingCmd struct {
	*Dv5Cmd
	Target *EnrOrEnodeFlag `ask:"<target>" help:"Target ENR/enode"`
}

func (c *Dv5PingCmd) Help() string {
	return "Run discv5-ping"
}

func (c *Dv5PingCmd) Run(ctx context.Context, args ...string) error {
	if c.Dv5State.Dv5Node == nil {
		return NoDv5Err
	}
	if err := c.Dv5State.Dv5Node.Ping(c.Target.Enode); err != nil {
		return fmt.Errorf("Failed to ping: %v", err)
	}
	c.Log.Infof("Successfully pinged")
	return nil
}

type Dv5ResolveCmd struct {
	*Dv5Cmd
	Target *EnrOrEnodeFlag `ask:"<target>" help:"Target ENR/enode"`
}

func (c *Dv5ResolveCmd) Help() string {
	return "Resolve target address and try to find latest record for it."
}

func (c *Dv5ResolveCmd) Run(ctx context.Context, args ...string) error {
	if c.Dv5State.Dv5Node == nil {
		return NoDv5Err
	}
	resolved := c.Dv5State.Dv5Node.Resolve(c.Target.Enode)
	if resolved != nil {
		return fmt.Errorf("Failed to resolve %s, nil result", c.Target.String())
	}
	c.Log.WithField("enr", resolved.String()).Infof("Successfully resolved")
	return nil
}

type Dv5RequestCmd struct {
	*Dv5Cmd
	Target *EnrOrEnodeFlag `ask:"<target>" help:"Target ENR/enode"`
}

func (c *Dv5RequestCmd) Help() string {
	return "Request target address directly."
}

func (c *Dv5RequestCmd) Run(ctx context.Context, args ...string) error {
	if c.Dv5State.Dv5Node == nil {
		return NoDv5Err
	}
	enrRes, err := c.Dv5State.Dv5Node.RequestENR(c.Target.Enode)
	if err != nil {
		return err
	}
	c.Log.WithField("enr", enrRes.String()).Infof("Successfully got ENR for node")
	return nil
}

type Dv5LookupCmd struct {
	*Dv5Cmd
	Target *NodeIDFlexibleFlag `ask:"<target>" help:"Target ENR/enode/node-id"`
}

func (c *Dv5LookupCmd) Help() string {
	return "Get list of nearby nodes. If no target node is provided, then find nodes nearby to self."
}

func (c *Dv5LookupCmd) Run(ctx context.Context, args ...string) error {
	if c.Dv5State.Dv5Node == nil {
		return NoDv5Err
	}
	if c.Target.ID == (enode.ID{}) {
		c.Target.ID = c.Dv5State.Dv5Node.Self().ID()
	}

	res := c.Dv5State.Dv5Node.Lookup(c.Target.ID)
	enrs := make([]string, 0, len(res))
	for _, v := range res {
		enrs = append(enrs, v.String())
	}
	c.Log.WithField("nodes", enrs).Infof("Lookup complete")
	return nil
}

type Dv5LookupRandomCmd struct {
	*Dv5Cmd
}

func (c *Dv5LookupRandomCmd) Help() string {
	return "Get random multi addrs, keep going until stopped"
}

func (c *Dv5LookupRandomCmd) Run(ctx context.Context, args ...string) error {
	if c.Dv5State.Dv5Node == nil {
		return NoDv5Err
	}
	randomNodes := c.Dv5State.Dv5Node.RandomNodes()
	log.Info("Started looking for random nodes")

	go func() {
		<-ctx.Done()
		randomNodes.Close()
	}()
	for {
		if !randomNodes.Next() {
			break
		}
		res := randomNodes.Node()
		c.Log.WithField("node", res.String()).Infof("Got random node")
	}
	log.Info("Stopped looking for random nodes")
	return nil
}

type Dv5SelfCmd struct {
	*Dv5Cmd
}

func (c *Dv5SelfCmd) Help() string {
	return "get local discv5 ENR"
}

func (c *Dv5SelfCmd) Run(ctx context.Context, args ...string) error {
	if c.Dv5State.Dv5Node == nil {
		return NoDv5Err
	}
	c.Log.WithField("enr", c.Dv5State.Dv5Node.Self()).Infof("local dv5 node")
	return nil
}
