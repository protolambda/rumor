package actor

import (
	"context"
	"github.com/protolambda/ask"
	chaindata "github.com/protolambda/rumor/chain"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/blocks"
	"github.com/protolambda/rumor/control/actor/chain"
	"github.com/protolambda/rumor/control/actor/dv5"
	"github.com/protolambda/rumor/control/actor/enr"
	"github.com/protolambda/rumor/control/actor/gossip"
	"github.com/protolambda/rumor/control/actor/host"
	"github.com/protolambda/rumor/control/actor/peer"
	"github.com/protolambda/rumor/control/actor/peer/metadata"
	"github.com/protolambda/rumor/control/actor/peer/status"
	"github.com/protolambda/rumor/control/actor/peerstore"
	"github.com/protolambda/rumor/control/actor/rpc"
	"github.com/protolambda/rumor/control/actor/states"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/sirupsen/logrus"
	"time"
)

type PeerStore struct {
}

func (ps *PeerStore) Agent() {
	// TODO

}

type ActorID string

type Actor struct {
	ID ActorID

	GlobalPeerstores track.Peerstores
	CurrentPeerstore track.DynamicPeerstore

	PeerStatusState   status.PeerStatusState
	PeerMetadataState metadata.PeerMetadataState

	GlobalChains chaindata.Chains
	ChainState   chain.ChainState

	// TODO init databases
	Blocks bdb.DB
	States sdb.DB

	Dv5State dv5.Dv5State

	GossipState gossip.GossipState
	RPCState    rpc.RPCState

	HostState host.HostState

	GlobalCtx context.Context

	ActorCtx    context.Context
	actorCancel context.CancelFunc
}

func NewActor(globalCtx context.Context, id ActorID) *Actor {
	ctxAll, cancelAll := context.WithCancel(globalCtx)
	act := &Actor{
		GlobalCtx:        globalCtx,
		ID:               id,
		ActorCtx:         ctxAll,
		actorCancel:      cancelAll,
		CurrentPeerstore: track.NewDynamicPeerstore(),
		Blocks:           &bdb.MemDB{},
		States:           &sdb.MemDB{},
	}
	return act
}

func (r *Actor) Close() {
	r.actorCancel()
}

func (r *Actor) MakeCmd(log logrus.FieldLogger, spawnCtx base.SpawnFn) *ActorCmd {
	return &ActorCmd{
		Actor:        r,
		Log:          log,
		SpawnContext: spawnCtx,
	}
}

type ActorCmd struct {
	*Actor
	// To log things while the command runs, including spawned processes
	Log logrus.FieldLogger
	// For non-blocking tasks to end later. E.g. serving data in the background.
	SpawnContext base.SpawnFn
}

func (c *ActorCmd) Cmd(route string) (cmd interface{}, err error) {
	b := &base.Base{
		WithHost:      &c.HostState,
		GlobalContext: c.GlobalCtx,
		ActorContext:  c.ActorCtx,
		SpawnContext:  c.SpawnContext,
		Log:           c.Log,
	}
	switch route {
	case "host":
		cmd = &host.HostCmd{
			Base:             b,
			WithSetHost:      &c.HostState,
			WithSetEnr:       &c.HostState,
			WithCloseHost:    &c.HostState,
			GlobalPeerstores: &c.GlobalPeerstores,
			CurrentPeerstore: c.CurrentPeerstore,
		}
	case "enr":
		cmd = &enr.EnrCmd{Base: b}
	case "peer":
		store := c.CurrentPeerstore
		if !store.Initialized() {
			store = nil
		}
		cmd = &peer.PeerCmd{
			Base:              b,
			PeerStatusState:   &c.PeerStatusState,
			PeerMetadataState: &c.PeerMetadataState,
			Store:             store,
		}
	case "peerstore":
		cmd = &peerstore.PeerstoreCmd{
			Base:             b,
			GlobalPeerstores: &c.GlobalPeerstores,
			CurrentPeerstore: c.CurrentPeerstore,
		}
	case "dv5":
		cmd = &dv5.Dv5Cmd{Base: b, Dv5State: &c.Dv5State, WithPriv: &c.HostState, Store: c.CurrentPeerstore}
	case "gossip":
		cmd = &gossip.GossipCmd{Base: b, GossipState: &c.GossipState}
	case "rpc":
		cmd = &rpc.RpcCmd{Base: b, RPCState: &c.RPCState}
	case "blocks":
		cmd = &blocks.BlocksCmd{Base: b, DB: c.Blocks}
	case "states":
		cmd = &states.StatesCmd{Base: b, DB: c.States}
	case "chain":
		cmd = &chain.ChainCmd{Base: b, Chains: &c.GlobalChains,
			ChainState: &c.ChainState, Blocks: c.Blocks, States: c.States}
	case "sleep":
		cmd = &SleepCmd{Base: b}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

var topRoutes = []string{"host", "enr", "peer", "peerstore", "dv5", "gossip",
	"rpc", "blocks", "states", "chain", "sleep"}
var topRoutesMap = map[string]struct{}{}

func init() {
	for _, r := range topRoutes {
		topRoutesMap[r] = struct{}{}
	}
}

func (c *ActorCmd) Routes() []string {
	return topRoutes
}

func IsActorCmd(args []string) bool {
	if len(args) == 0 {
		return false
	}
	_, ok := topRoutesMap[args[0]]
	return ok
}

func (c *ActorCmd) Help() string {
	return "Interact with any p2p component in Eth2"
}

type SleepCmd struct {
	*base.Base
	Time time.Duration `ask:"<time>" help:"How long to sleep, e.g. 3s"`
}

func (c *SleepCmd) Help() string {
	return "Sleep for given amount of time."
}

func (c *SleepCmd) Default() {
	c.Time = time.Second
}

func (c *SleepCmd) Run(ctx context.Context, args ...string) error {
	if c.Time != 0 {
		c.Log.Infof("sleeping %s", c.Time.String())
		ticker := time.NewTicker(c.Time)
		select {
		case <-ticker.C:
			c.Log.Infoln("done sleeping!")
			break
		case <-ctx.Done():
			c.Log.Infof("stopped sleep, exit early: %v", ctx.Err())
			break
		}
		ticker.Stop()
	} else {
		c.Log.Warn("cannot sleep for 0 duration")
	}
	return nil
}
