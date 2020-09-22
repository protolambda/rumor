package actor

import (
	"context"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p-core/crypto"
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
	"github.com/protolambda/rumor/control/tool"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/sirupsen/logrus"
	"io"
	"sync"
	"time"
)

type GlobalActorData struct {
	GlobalCtx        context.Context
	GlobalPeerstores track.Peerstores
	GlobalChains     chaindata.Chains
	GlobalBlocksDBs  bdb.DBs
	GlobalStatesDBs  sdb.DBs
}

type ActorID string

type Actor struct {
	ID ActorID

	*GlobalActorData

	CurrentPeerstore track.DynamicPeerstore

	PeerStatusState   status.PeerStatusState
	PeerMetadataState metadata.PeerMetadataState

	ChainState  chain.ChainState
	BlocksState blocks.DBState
	StatesState states.DBState

	Dv5State dv5.Dv5State

	GossipState gossip.GossipState
	RPCState    rpc.RPCState

	HostState host.HostState

	LazyEnrState enr.LazyEnrState

	privLock sync.Mutex
	priv     *crypto.Secp256k1PrivateKey

	ActorCtx    context.Context
	actorCancel context.CancelFunc
}

func NewActor(id ActorID, globals *GlobalActorData) *Actor {
	ctxAll, cancelAll := context.WithCancel(globals.GlobalCtx)
	act := &Actor{
		GlobalActorData:  globals,
		ID:               id,
		ActorCtx:         ctxAll,
		actorCancel:      cancelAll,
		CurrentPeerstore: track.NewDynamicPeerstore(),
	}
	return act
}

func (r *Actor) Close() {
	r.actorCancel()
}

func (r *Actor) GetPriv() *crypto.Secp256k1PrivateKey {
	r.privLock.Lock()
	defer r.privLock.Unlock()
	// TODO: could do more here, but ok to return nil
	return r.priv
}

func (r *Actor) SetPriv(priv *crypto.Secp256k1PrivateKey) error {
	r.privLock.Lock()
	defer r.privLock.Unlock()
	if r.priv != nil {
		if ok, err := addrutil.PrivKeysEqual(r.priv, priv); err != nil {
			return fmt.Errorf("failed priv key conflict check: %v", err)
		} else if !ok {
			return errors.New("cannot change private key")
		}
	}
	r.priv = priv
	return nil
}

func (r *Actor) GetNode() (n *enode.Node, ok bool) {
	if current := r.LazyEnrState.Current; current != nil {
		return current.GetNode(), true
	} else {
		return nil, false
	}
}

func (r *Actor) MakeCmd(log logrus.FieldLogger, control base.Control, out io.Writer) *ActorCmd {
	return &ActorCmd{
		Actor:   r,
		Log:     log,
		Control: control,
		Out:     out,
	}
}

type ActorCmd struct {
	*Actor
	// To log things while the command runs, including spawned processes
	Log logrus.FieldLogger
	// Control the execution of a command
	Control base.Control
	// Optional std output
	Out io.Writer
}

func (c *ActorCmd) Help() string {
	return `Rumor provides a suite of commands for p2p/eth2 actions.
Each action can (optionally):
- Run by one actor, e.g. 'alice: host start'
- Run with an ID, e.g. '_my_reference host start'
- Run with a log level, e.g. 'lvl_warn host start' (each level is prefixed with lvl_)

Log data fields automatically become available. E.g. '_my_reference_enr' to get 'enr' field, or '__enr' when it's from the last Rumor command.

And Bash syntax can be used to script control flow.
Rumor is a shell, but restricts unknown commands to running as a sub-process.
`
}

func (c *ActorCmd) Cmd(route string) (cmd interface{}, err error) {
	b := &base.Base{
		WithHost:      &c.HostState,
		GlobalContext: c.GlobalCtx,
		ActorContext:  c.ActorCtx,
		Control:       c.Control,
		Log:           c.Log,
		Out:           c.Out,
	}
	switch route {
	case "host":
		cmd = &host.HostCmd{
			Base:             b,
			WithSetHost:      &c.HostState,
			PrivSettings:     c,
			WithEnrNode:      c,
			WithCloseHost:    &c.HostState,
			GlobalPeerstores: c.GlobalPeerstores,
			CurrentPeerstore: c.CurrentPeerstore,
		}
	case "enr":
		cmd = &enr.EnrCmd{Base: b, Lazy: &c.LazyEnrState, PrivSettings: c, WithHostPriv: &c.HostState}
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
			GlobalPeerstores: c.GlobalPeerstores,
			CurrentPeerstore: c.CurrentPeerstore,
		}
	case "dv5":
		settings := c.LazyEnrState.Current
		if settings == nil {
			return nil, errors.New("Discv5 needs an ENR first. Use 'enr make'.")
		}
		cmd = &dv5.Dv5Cmd{Base: b, Dv5State: &c.Dv5State, Dv5Settings: settings, CurrentPeerstore: c.CurrentPeerstore}
	case "gossip":
		cmd = &gossip.GossipCmd{Base: b, GossipState: &c.GossipState}
	case "rpc":
		cmd = &rpc.RpcCmd{Base: b, RPCState: &c.RPCState}
	case "blocks":
		cmd = &blocks.BlocksCmd{Base: b, DBs: c.GlobalBlocksDBs, DBState: &c.BlocksState}
	case "states":
		cmd = &states.StatesCmd{Base: b, DBs: c.GlobalStatesDBs, DBState: &c.StatesState}
	case "chain":
		bl, ok := c.GlobalBlocksDBs.Find(c.BlocksState.CurrentDB)
		if !ok {
			return nil, errors.New("no blocks DB available, try 'blocks create'")
		}
		st, ok := c.GlobalStatesDBs.Find(c.StatesState.CurrentDB)
		if !ok {
			return nil, errors.New("no states DB available, try 'states create'")
		}
		cmd = &chain.ChainCmd{Base: b, Chains: c.GlobalChains,
			ChainState: &c.ChainState, Blocks: bl, States: st}
	case "sleep":
		cmd = &SleepCmd{Base: b}
	case "tool":
		cmd = &tool.ToolCmd{Out: b.Out}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

var topRoutes = []string{"host", "enr", "peer", "peerstore", "dv5", "gossip",
	"rpc", "blocks", "states", "chain", "sleep", "tool"}
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
	if args[0] == "help" { // implicit command
		return true
	}
	_, ok := topRoutesMap[args[0]]
	return ok
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
