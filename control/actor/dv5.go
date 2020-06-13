package actor

import (
	"context"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/rumor/p2p/peering/dv5"
	"github.com/sirupsen/logrus"
)

type Dv5State struct {
	Dv5Node dv5.Discv5
}

type Dv5Cmd struct {
	*Actor    `ask:"-"`
	log       logrus.FieldLogger
	*Dv5State `ask:"-"`
}

func (c *Dv5Cmd) Get(ctx context.Context, args ...string) (cmd interface{}, remaining []string, err error) {
	if len(args) == 0 {
		return nil, nil, errors.New("no subcommand specified")
	}
	// TODO: when initializing a command, init the Target field
	switch args[0] {
	case "sleep":
		cmd = &DebugSleepCmd{
			Actor: c.Actor,
			log:   c.log,
		}
	default:
		return nil, args, fmt.Errorf("unrecognized command: %v", args)
	}
	return cmd, args[1:], nil
}

func (c *Dv5Cmd) Help() string {
	return "For debugging purposes" // TODO list subcommands
}

type Dv5StartCmd struct {
	*Actor `ask:"-"`
	log    logrus.FieldLogger
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
	c.Dv5State.Dv5Node, err = dv5.NewDiscV5(c.log, c.IP, c.UdpPort, c.PrivKey, bootNodes)
	if err != nil {
		return err
	}
	log.Info("Started discv5")
	return nil
}

var NoDv5Err = errors.New("Must start discv5 first. Try 'dv5 start'")

type Dv5StopCmd struct {
	*Actor `ask:"-"`
	log    logrus.FieldLogger
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
	*Actor `ask:"-"`
	log    logrus.FieldLogger
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
	c.log.Infof("Successfully pinged")
	return nil
}

type Dv5ResolveCmd struct {
	*Actor `ask:"-"`
	log    logrus.FieldLogger
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
	c.log.WithField("enr", resolved.String()).Infof("Successfully resolved")
	return nil
}

type Dv5RequestCmd struct {
	*Actor `ask:"-"`
	log    logrus.FieldLogger
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
	c.log.WithField("enr", enrRes.String()).Infof("Successfully got ENR for node")
	return nil
}

type Dv5LookupCmd struct {
	*Actor `ask:"-"`
	log    logrus.FieldLogger
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
	c.log.WithField("nodes", enrs).Infof("Lookup complete")
	return nil
}

type Dv5LookupRandomCmd struct {
	*Actor `ask:"-"`
	log    logrus.FieldLogger
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
		c.log.WithField("node", res.String()).Infof("Got random node")
	}
	log.Info("Stopped looking for random nodes")
	return nil
}

type Dv5SelfCmd struct {
	*Actor `ask:"-"`
	log    logrus.FieldLogger
}

func DefaultDv5SelfCmd(a *Actor, log logrus.FieldLogger) *Dv5SelfCmd {
	return &Dv5SelfCmd{Actor: a, log: log}
}

func (c *Dv5SelfCmd) Help() string {
	return "get local discv5 ENR"
}

func (c *Dv5SelfCmd) Run(ctx context.Context, args ...string) error {
	if c.Dv5State.Dv5Node == nil {
		return NoDv5Err
	}
	c.log.WithField("enr", c.Dv5State.Dv5Node.Self()).Infof("local dv5 node")
	return nil
}
