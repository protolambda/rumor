package repl

import (
	"eth2-lurk/node"
	"github.com/spf13/cobra"
)

var (
	HostCmd,
	EnrCmd,
	PeerCmd,
	Dv5Cmd,
	KadCmd,
	GossipCmd,
	RpcCmd *cobra.Command
)

func initCommands(getHostNode func() *node.Node) {
	HostCmd = &cobra.Command{
		Use: "host",
		Short: "Manage host",
	}
	HostCmd.AddCommand(&cobra.Command{
		Use:   "start [--priv-key=...] [--security=<secio|tls|noise>] [--mux=<yamux-mplex|mplex|yamux>] [--transports=<tcp,ws>]",
		Short: "Start the host node",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	HostCmd.AddCommand(&cobra.Command{
		Use:   "listen [multi-addr [multi-addr [...]]]",
		Short: "Start listening on given addresses",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	HostCmd.AddCommand(&cobra.Command{
		Use:   "view",
		Short: "View local peer ID, listening addresses, etc.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	EnrCmd = &cobra.Command{
		Use: "enr",
		Short: "Ethereum Name Record (ENR) utilities",
	}
	EnrCmd.AddCommand(&cobra.Command{
		Use:   "view <enr url-base64 (RFC 4648)>",
		Short: "view ENR contents",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	EnrCmd.AddCommand(&cobra.Command{
		Use:   "from <multi addr>",
		Short: "create ENR (encoded in url-base64 (RFC 4648)) from multi addr",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})

	PeerCmd = &cobra.Command{
		Use: "peer",
		Short: "Manage Libp2p peerstore",
	}
	PeerCmd.AddCommand(&cobra.Command{
		Use: "list <all | connected>",
		Short: "List peers in peerstore",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	PeerCmd.AddCommand(&cobra.Command{
		Use: "prune <N connections to keep>",
		Short: "Prune peers",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	PeerCmd.AddCommand(&cobra.Command{
		Use: "limit <lo peer count> <hi peer count>",
		Short: "Prune down to <lo> whenever peer count exceeds <hi>",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	PeerCmd.AddCommand(&cobra.Command{
		Use: "connect <multi addr> [tag]",
		Short: "Connect to peer",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	PeerCmd.AddCommand(&cobra.Command{
		Use: "disconnect <peerID or multi addr>",
		Short: "Disconnect peer",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	PeerCmd.AddCommand(&cobra.Command{
		Use: "protect <peerID> <tag>",
		Short: "Protect peer, tagging them as <tag>",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	PeerCmd.AddCommand(&cobra.Command{
		Use: "unprotect <peerID> <tag>",
		Short: "Unprotect peer, un-tagging them as <tag>",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	PeerCmd.AddCommand(&cobra.Command{
		Use: "addrs <peerID>",
		Short: "View known addresses of <peerID>",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})

	Dv5Cmd = &cobra.Command{
		Use: "dv5",
		Short: "Manage Ethereum Discv5",
	}
	Dv5Cmd.AddCommand(&cobra.Command{
		Use: "start [--addr=UDP-addr or --enr=ENR-addr] [--priv-key=priv key]",
		Short: "Start discv5 discovery",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	Dv5Cmd.AddCommand(&cobra.Command{
		Use: "stop [--addr=... or --enr=... or --pubkey=...]",
		Short: "Stop discv5 discovery on given address",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	Dv5Cmd.AddCommand(&cobra.Command{
		Use: "search",
		Short: "Actively query for new discv5 nodes",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})

	KadCmd = &cobra.Command{
		Use: "kad",
		Short: "Manage Libp2p Kademlia DHT",
	}
	KadCmd.AddCommand(&cobra.Command{
		Use: "start <protocol ID>",
		Short: "Go onto the given Kademlia DHT, if known by any connected peer (connect to bootnode first)",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	KadCmd.AddCommand(&cobra.Command{
		Use: "refresh [--wait]",
		Short: "Refresh the Kademlia table. Optionally wait for it to complete.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})

	GossipCmd = &cobra.Command{
		Use: "gossip",
		Short: "Manage Libp2p GossipSub",
	}
	GossipCmd.AddCommand(&cobra.Command{
		Use: "list",
		Short: "List joined gossip topics",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	GossipCmd.AddCommand(&cobra.Command{
		Use: "join <topic>",
		Short: "Join a gossip topic. Propagate anything.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	GossipCmd.AddCommand(&cobra.Command{
		Use: "lurk <topic>",
		Short: "Lurk a gossip topic. Propagate nothing.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	GossipCmd.AddCommand(&cobra.Command{
		Use: "leave <topic>",
		Short: "Leave a gossip topic.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	GossipCmd.AddCommand(&cobra.Command{
		Use: "log <topic> <output file path>",
		Short: "Log the messages of a gossip topic to a file. 1 hex-encoded message per line. Join/lurk a topic first.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})

	RpcCmd = &cobra.Command{
		Use: "rpc",
		Short: "Manage Eth2 RPC",
	}
	RpcCmd.AddCommand(&cobra.Command{
		Use: "list",
		Short: "List active RPC listeners",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	RpcCmd.AddCommand(&cobra.Command{
		Use: "log-req <method name> [compression]",
		Short: "Log requests to a file",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
	RpcCmd.AddCommand(&cobra.Command{
		Use: "goodbye <peerID> <code> [compression] [--disconnect]",
		Short: "Send a goodbye to a peer, optionally disconnecting the peer after sending the Goodbye.",
		Run: func(cmd *cobra.Command, args []string) {

		},
	})
}

