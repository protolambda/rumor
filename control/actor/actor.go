package actor

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/protolambda/rumor/chain"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/rumor/p2p/track"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"net"
	"sync"
)


func InitRootCmd(ctx context.Context, log logrus.FieldLogger) *cobra.Command {
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
		r.InitDv5Cmd(ctx, log),
		r.InitKadCmd(ctx, log),
		r.InitGossipCmd(ctx, log),
		r.InitRpcCmd(ctx, log),
	)
	return cmd
}
