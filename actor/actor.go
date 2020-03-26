package actor

import (
	"context"
	"crypto/ecdsa"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/protolambda/rumor/addrutil"
	"github.com/protolambda/rumor/node"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"net"
)

type Actor struct {
	P2PHost host.Host

	PrivKey crypto.PrivKey

	IP      net.IP
	TcpPort uint16
	UdpPort uint16

	Dv5State    Dv5State
	KadState    KadState
	GossipState GossipState
	RPCState    RPCState

	ActorCtx    context.Context
	actorCancel context.CancelFunc
}

// check interface
var _ = (node.Node)((*Actor)(nil))

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

func (r *Actor) Cmd(ctx context.Context, log *Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rumor",
		Short: "A REPL for Eth2 networking.",
		Long:  `A REPL for Eth2 networking. For debugging and interacting with Eth2 network components.`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	// TODO: if too slow to initialize all commands, we could initialize just the called command.
	cmd.AddCommand(
		r.IniDebugCmd(ctx, log),
		r.InitHostCmd(ctx, log),
		r.InitEnrCmd(ctx, log),
		r.InitPeerCmd(ctx, log),
		r.InitDv5Cmd(ctx, log, &r.Dv5State),
		r.InitKadCmd(ctx, log, &r.KadState),
		r.InitGossipCmd(ctx, log, &r.GossipState),
		r.InitRpcCmd(ctx, log, &r.RPCState),
	)
	return cmd
}

func (r *Actor) Host() host.Host {
	return r.P2PHost
}

// shortcut to check if there is a libp2p host available, and error-log if not available.
func (r *Actor) NoHost(log logrus.FieldLogger) bool {
	if r.P2PHost == nil {
		log.Error("REPL must have initialized Libp2p host. Try 'host start'")
		return true
	}
	return false
}

func (r *Actor) GetEnr() *enr.Record {
	priv := (*ecdsa.PrivateKey)(r.PrivKey.(*crypto.Secp256k1PrivateKey))
	return addrutil.MakeENR(r.IP, r.TcpPort, r.UdpPort, priv)
}
