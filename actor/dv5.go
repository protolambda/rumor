package actor

import (
	"context"
	"encoding/hex"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/protolambda/rumor/addrutil"
	"github.com/protolambda/rumor/peering/dv5"
	"github.com/protolambda/rumor/peering/static"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"time"
)

type Dv5State struct {
	Dv5Node  dv5.Discv5
	CloseDv5 context.CancelFunc
}

func (r *Actor) InitDv5Cmd(log logrus.FieldLogger, state *Dv5State) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dv5",
		Short: "Manage Ethereum Discv5",
	}

	noDv5 := func(cmd *cobra.Command) bool {
		if r.NoHost(log) {
			return true
		}
		if state.Dv5Node == nil {
			log.Error("REPL must have initialized discv5. Try 'dv5 start'")
			return true
		}
		return false
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "start [<bootstrap-addr> [...]]",
		Short: "Start discv5.",
		Long:  "Start discv5.",
		Args:  cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if r.NoHost(log) {
				return
			}
			if r.IP == nil {
				log.Error("Host has no IP yet. Get with 'host listen'")
				return
			}
			if state.Dv5Node != nil {
				log.Errorf("Already have dv5 open at %s", state.Dv5Node.Self().String())
				return
			}
			bootNodes := make([]*enode.Node, 0, len(args))
			for i := 1; i < len(args); i++ {
				dv5Addr, err := addrutil.ParseEnodeAddr(args[i])
				if err != nil {
					log.Error(err)
					return
				}
				bootNodes = append(bootNodes, dv5Addr)
			}
			ctx, cancel := context.WithCancel(r.Ctx)
			var err error
			state.Dv5Node, err = dv5.NewDiscV5(ctx, log, r, r.IP, r.UdpPort, r.PrivKey, bootNodes)
			if err != nil {
				log.Error(err)
				return
			}
			state.CloseDv5 = cancel
			log.Info("Started discv5")
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop discv5",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			state.CloseDv5()
			state.Dv5Node = nil
			log.Info("Stopped discv5")
		},
	})

	handleLookup := func(nodes []*enode.Node, connect bool) error {
		dv5AddrsOut := ""
		for i, v := range nodes {
			if i > 0 {
				dv5AddrsOut += "  "
			}
			dv5AddrsOut += v.String()
		}
		mAddrs, err := addrutil.EnodesToMultiAddrs(nodes)
		if err != nil {
			return err
		}
		mAddrsOut := ""
		for i, v := range mAddrs {
			if i > 0 {
				mAddrsOut += "  "
			}
			mAddrsOut += v.String()
		}
		log.Infof("addresses of nodes (%d): %s", len(mAddrs), mAddrsOut)
		if connect {
			log.Infof("connecting to nodes (with a 10 second timeout)")
			ctx, _ := context.WithTimeout(r.Ctx, time.Second*10)
			err := static.ConnectStaticPeers(ctx, log, r, mAddrs, func(info peer.AddrInfo, alreadyConnected bool) error {
				log.Infof("connected to peer from discv5 nodes: %s", info.String())
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "ping <target node: enode address or ENR (url-base64)>",
		Short: "Run discv5-ping",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			target, err := addrutil.ParseEnodeAddr(args[0])
			if err != nil {
				log.Error(err)
			}
			if err := state.Dv5Node.Ping(target); err != nil {
				log.Errorf("Failed to ping %s: %v", target.String(), err)
				return
			}
			log.Infof("Successfully pinged %s: ", target.String())
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "resolve <target node: enode address or ENR (url-base64)>",
		Short: "Resolve target address and try to find latest record for it.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			target, err := addrutil.ParseEnodeAddr(args[0])
			if err != nil {
				log.Error(err)
			}
			resolved := state.Dv5Node.Resolve(target)
			if resolved != nil {
				log.Errorf("Failed to resolve %s, nil result", target.String())
				return
			}
			log.Infof("Successfully resolved:   %s   -->  %s", target.String(), resolved.String())
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "get-enr <target node: enode address or ENR (url-base64)>",
		Short: "Resolve target address and try to find latest record for it.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			target, err := addrutil.ParseEnodeAddr(args[0])
			if err != nil {
				log.Error(err)
			}
			enrRes, err := state.Dv5Node.RequestENR(target)
			if err != nil {
				log.Error(err)
				return
			}
			log.Infof("Successfully got ENR for node:   %s   -->  %s", target.String(), enrRes.String())
		},
	})

	connectLookup := false
	lookupCmd := &cobra.Command{
		Use:   "lookup [target node: hex node ID, enode address or ENR (url-base64)]",
		Short: "Get list of nearby multi addrs. If no target node is provided, then find nodes nearby to self.",
		Args:  cobra.RangeArgs(0, 1),
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			target := state.Dv5Node.Self().ID()
			if len(args) > 0 {
				if n, err := addrutil.ParseEnodeAddr(args[0]); err != nil {
					if h, err := hex.DecodeString(args[0]); err != nil {
						log.Error("provided target node is not a valid node ID, enode address or ENR")
						return
					} else {
						if len(h) != 32 {
							log.Error("hex node ID is not 32 bytes")
							return
						} else {
							copy(target[:], h)
						}
					}
				} else {
					target = n.ID()
				}
			}
			res := state.Dv5Node.Lookup(target)
			if err := handleLookup(res, connectLookup); err != nil {
				log.Error(err)
				return
			}
		},
	}
	lookupCmd.Flags().BoolVar(&connectLookup, "connect", false, "Connect to the resulting nodes")
	cmd.AddCommand(lookupCmd)

	connectRandom := false
	randomCommand := &cobra.Command{
		Use:   "lookup-random",
		Short: "Get list of random multi addrs.",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			res := state.Dv5Node.LookupRandom()
			if err := handleLookup(res, connectRandom); err != nil {
				log.Error(err)
				return
			}
		},
	}
	randomCommand.Flags().BoolVar(&connectRandom, "connect", false, "Connect to the resulting nodes")
	cmd.AddCommand(randomCommand)

	cmd.AddCommand(&cobra.Command{
		Use:   "self",
		Short: "get local discv5 ENR",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			v, err := addrutil.EnrToString(r.GetEnr())
			if err != nil {
				log.Error(err)
				return
			}
			log.Infof("local ENR: %s", v)
			enodeAddr := state.Dv5Node.Self()
			log.Infof("local dv5 node (no TCP in ENR): %s", enodeAddr.String())
		},
	})
	return cmd
}

