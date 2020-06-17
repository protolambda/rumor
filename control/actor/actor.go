package actor

import (
	"context"
	chaindata "github.com/protolambda/rumor/chain"
	"github.com/protolambda/rumor/control/actor/chain"
	"github.com/protolambda/rumor/control/actor/dv5"
	"github.com/protolambda/rumor/control/actor/gossip"
	"github.com/protolambda/rumor/control/actor/peer/metadata"
	"github.com/protolambda/rumor/control/actor/peer/status"
	"github.com/protolambda/rumor/control/actor/rpc"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/sirupsen/logrus"
)

type Actor struct {
	GlobalPeerInfos   track.PeerInfos
	PeerStatusState   status.PeerStatusState
	PeerMetadataState metadata.PeerMetadataState

	GlobalChains chaindata.Chains
	ChainState   chain.ChainState

	Dv5State    dv5.Dv5State
	GossipState gossip.GossipState
	RPCState    rpc.RPCState

	ActorCtx    context.Context
	actorCancel context.CancelFunc
}

func NewActor() *Actor {
	ctxAll, cancelAll := context.WithCancel(context.Background())
	return &Actor{
		ActorCtx:    ctxAll,
		actorCancel: cancelAll,
	}
}

func (r *Actor) Close() {
	r.actorCancel()
}

func InitRootCmd(ctx context.Context, log logrus.FieldLogger) {
	//cmd := &Command{
	//	Use:   "rumor",
	//	Short: "A REPL for Eth2 networking.",
	//	Long:  `A REPL for Eth2 networking. For debugging and interacting with Eth2 network components.`,
	//	Run: func(cmd *cobra.Command, args []string) {
	//		_ = cmd.Help()
	//	},
	//}
	// TODO: if too slow to initialize all commands, we could initialize just the called command.
	//cmd.AddCommand(
	//	r.IniDebugCmd(ctx, log),
	//	r.InitHostCmd(ctx, log),
	//	r.InitEnrCmd(ctx, log),
	//	r.InitPeerCmd(ctx, log),
	//	r.InitDv5Cmd(ctx, log),
	//	r.InitKadCmd(ctx, log),
	//	r.InitGossipCmd(ctx, log),
	//	r.InitRpcCmd(ctx, log),
	//)
	//return cmd
}
