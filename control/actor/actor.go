package actor

import (
	"context"
	libpeer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/ask"
	chaindata "github.com/protolambda/rumor/chain"
	bdb "github.com/protolambda/rumor/chain/db/blocks"
	sdb "github.com/protolambda/rumor/chain/db/states"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/blocks"
	"github.com/protolambda/rumor/control/actor/chain"
	"github.com/protolambda/rumor/control/actor/debug"
	"github.com/protolambda/rumor/control/actor/dv5"
	"github.com/protolambda/rumor/control/actor/enr"
	"github.com/protolambda/rumor/control/actor/gossip"
	"github.com/protolambda/rumor/control/actor/host"
	"github.com/protolambda/rumor/control/actor/peer"
	"github.com/protolambda/rumor/control/actor/peer/metadata"
	"github.com/protolambda/rumor/control/actor/peer/status"
	"github.com/protolambda/rumor/control/actor/rpc"
	"github.com/protolambda/rumor/control/actor/states"
	"github.com/protolambda/rumor/p2p/rpc/methods"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/sirupsen/logrus"
)

type PeerStore struct {

}

func (ps *PeerStore) Agent() {
	// TODO

}

type Actor struct {
	GlobalPeerstores   track.Peerstores
	CurrentPeerstore  track.PeerstoreID

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

	ActorCtx    context.Context
	actorCancel context.CancelFunc
}

func NewActor() *Actor {
	ctxAll, cancelAll := context.WithCancel(context.Background())
	act := &Actor{
		ActorCtx:    ctxAll,
		actorCancel: cancelAll,
		Blocks:      &bdb.MemDB{},
		States:      &sdb.MemDB{},
	}
	act.PeerStatusState.OnStatus = func(peerID libpeer.ID, status *methods.Status) {
		inf, _ := act.GlobalPeerInfos.Find(peerID)
		inf.RegisterStatus(*status)
	}
	act.PeerMetadataState.OnMetadata = func(peerID libpeer.ID, meta *methods.MetaData) {
		inf, _ := act.GlobalPeerInfos.Find(peerID)
		inf.RegisterMetadata(*meta)
	}
	act.PeerMetadataState.IsInteresting = func(peerID libpeer.ID, seqNr methods.SeqNr, maxTries uint64) bool {
		inf, _ := act.GlobalPeerInfos.Find(peerID)
		return inf.ClaimedSeq() < seqNr
	}
	return act
}

func (r *Actor) Close() {
	r.actorCancel()
}

func (r *Actor) MakeCmd(log logrus.FieldLogger) *ActorCmd {
	return &ActorCmd{
		Actor: r,
		Log:   log,
	}
}

type ActorCmd struct {
	*Actor
	Log logrus.FieldLogger
}

func (c *ActorCmd) Cmd(route string) (cmd interface{}, err error) {
	b := &base.Base{
		WithHost:    &c.HostState,
		BaseContext: c.ActorCtx,
		Log:         c.Log,
	}
	switch route {
	case "host":
		cmd = &host.HostCmd{
			Base:          b,
			WithSetHost:   &c.HostState,
			WithSetEnr:    &c.HostState,
			WithCloseHost: &c.HostState,
		}
	case "enr":
		cmd = &enr.EnrCmd{Base: b}
	case "peer":
		cmd = &peer.PeerCmd{
			Base:              b,
			PeerStatusState:   &c.PeerStatusState,
			PeerMetadataState: &c.PeerMetadataState,
			WithPeerInfos:     &c.GlobalPeerInfos,
		}
	case "dv5":
		cmd = &dv5.Dv5Cmd{Base: b, Dv5State: &c.Dv5State, WithPriv: &c.HostState, PeerInfoFinder: &c.GlobalPeerInfos}
	case "gossip":
		cmd = &gossip.GossipCmd{Base: b, GossipState: &c.GossipState}
	case "rpc":
		cmd = &rpc.RpcCmd{Base: b, RPCState: &c.RPCState}
	case "debug":
		cmd = &debug.DebugCmd{Base: b}
	case "blocks":
		cmd = &blocks.BlocksCmd{Base: b, DB: c.Blocks}
	case "states":
		cmd = &states.StatesCmd{Base: b, DB: c.States}
	case "chain":
		cmd = &chain.ChainCmd{Base: b, Chains: &c.GlobalChains,
			ChainState: &c.ChainState, Blocks: c.Blocks, States: c.States}
	default:
		return nil, ask.UnrecognizedErr
	}
	return cmd, nil
}

func (c *ActorCmd) Routes() []string {
	return []string{"host", "enr", "peer", "dv5", "gossip", "rpc", "debug"}
}

func (c *ActorCmd) Help() string {
	return "Interact with any p2p component in Eth2"
}
