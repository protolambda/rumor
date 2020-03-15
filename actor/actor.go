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

	Ctx    context.Context
	Cancel context.CancelFunc
}

// check interface
var _ = (node.Node)((*Actor)(nil))

func NewActor() *Actor {
	ctxAll, cancelAll := context.WithCancel(context.Background())
	return &Actor{
		Ctx: ctxAll,
		Cancel: cancelAll,
	}
}

func (r *Actor) Cmd(log *Logger) *cobra.Command {
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
		r.InitHostCmd(log),
		r.InitEnrCmd(log),
		r.InitPeerCmd(log),
		r.InitDv5Cmd(log, &r.Dv5State),
		r.InitKadCmd(log, &r.KadState),
		r.InitGossipCmd(log, &r.GossipState),
		r.InitRpcCmd(log, &r.RPCState),
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
