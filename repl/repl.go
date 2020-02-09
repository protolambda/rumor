package repl

import (
	"context"
	"eth2-lurk/node"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type Repl struct {
	HostNode *node.Node
	Log   logrus.FieldLogger
	Ctx   context.Context
	Cancel func()
	ReplCmd *cobra.Command
}

func NewRepl(log logrus.FieldLogger) *Repl {
	repl := &Repl{
		Log: log,
	}
	{
		ctxAll, cancelAll := context.WithCancel(context.Background())
		repl.Ctx = ctxAll
		repl.Cancel = cancelAll
	}
	cmd := &cobra.Command{
		Use:   "eth2-net-repl",
		Short: "A REPL for Eth2 networking.",
		Long:  `A REPL for Eth2 networking. For debugging and interacting with Eth2 network components.`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	cmd.AddCommand(
		repl.InitHostCmd(),
		repl.InitEnrCmd(),
		repl.InitPeerCmd(),
		repl.InitDv5Cmd(),
		repl.InitKadCmd(),
		repl.InitGossipCmd(),
		repl.InitRpcCmd(),
	)
	repl.ReplCmd = cmd
	return repl
}

func writeErr(cmd *cobra.Command, format string, a ...interface{}) {
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), format, a)
}

func (r *Repl) InitHostCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "host",
		Short: "Manage host",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "start [--priv-key=...] [--security=<secio|tls|noise>] [--mux=<yamux-mplex|mplex|yamux>] [--transports=<tcp,ws>]",
		Short: "Start the host node",
		Run: func(cmd *cobra.Command, args []string) {
			if r.HostNode != nil {
				writeErr(cmd, "Already have a host open.")
				return
			}

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "listen [multi-addr [multi-addr [...]]]",
		Short: "Start listening on given addresses",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "view",
		Short: "View local peer ID, listening addresses, etc.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	return cmd
}

func (r *Repl) InitEnrCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enr",
		Short: "Ethereum Name Record (ENR) utilities",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "view <enr url-base64 (RFC 4648)>",
		Short: "view ENR contents",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "from <multi addr>",
		Short: "create ENR (encoded in url-base64 (RFC 4648)) from multi addr",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	return cmd
}

func (r *Repl) InitPeerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "peer",
		Short: "Manage Libp2p peerstore",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list <all | connected>",
		Short: "List peers in peerstore",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "prune <N connections to keep>",
		Short: "Prune peers",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "limit <lo peer count> <hi peer count>",
		Short: "Prune down to <lo> whenever peer count exceeds <hi>",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "connect <multi addr> [tag]",
		Short: "Connect to peer",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "disconnect <peerID or multi addr>",
		Short: "Disconnect peer",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "protect <peerID> <tag>",
		Short: "Protect peer, tagging them as <tag>",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "unprotect <peerID> <tag>",
		Short: "Unprotect peer, un-tagging them as <tag>",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "addrs <peerID>",
		Short: "View known addresses of <peerID>",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	return cmd
}

func (r *Repl) InitDv5Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dv5",
		Short: "Manage Ethereum Discv5",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "start [--addr=UDP-addr or --enr=ENR-addr] [--priv-key=priv key]",
		Short: "Start discv5 discovery",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "stop [--addr=... or --enr=... or --pubkey=...]",
		Short: "Stop discv5 discovery on given address",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "search",
		Short: "Actively query for new discv5 nodes",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	return cmd
}

func (r *Repl) InitKadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kad",
		Short: "Manage Libp2p Kademlia DHT",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "start <protocol ID>",
		Short: "Go onto the given Kademlia DHT, if known by any connected peer (connect to bootnode first)",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "refresh [--wait]",
		Short: "Refresh the Kademlia table. Optionally wait for it to complete.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	return cmd
}

func (r *Repl) InitGossipCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gossip",
		Short: "Manage Libp2p GossipSub",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List joined gossip topics",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "join <topic>",
		Short: "Join a gossip topic. Propagate anything.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "lurk <topic>",
		Short: "Lurk a gossip topic. Propagate nothing.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "leave <topic>",
		Short: "Leave a gossip topic.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "log <topic> <output file path>",
		Short: "Log the messages of a gossip topic to a file. 1 hex-encoded message per line. Join/lurk a topic first.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	return cmd
}

func (r *Repl) InitRpcCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "rpc",
		Short: "Manage Eth2 RPC",
	}
	cmd.AddCommand(&cobra.Command{
		Use: "list",
		Short: "List active RPC listeners",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "log-req <method name> [compression]",
		Short: "Log requests to a file",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "goodbye <peerID> <code> [compression] [--disconnect]",
		Short: "Send a goodbye to a peer, optionally disconnecting the peer after sending the Goodbye.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	return cmd
}

