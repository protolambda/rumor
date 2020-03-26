package actor

import (
	"context"
	"encoding/hex"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/protolambda/rumor/addrutil"
	"github.com/protolambda/rumor/peering/dv5"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type Dv5State struct {
	Dv5Node dv5.Discv5
}

func (r *Actor) InitDv5Cmd(ctx context.Context, log logrus.FieldLogger, state *Dv5State) *cobra.Command {
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
				dv5Addr, err := addrutil.ParseEnrOrEnode(args[i])
				if err != nil {
					log.Error(err)
					return
				}
				bootNodes = append(bootNodes, dv5Addr)
			}
			var err error
			state.Dv5Node, err = dv5.NewDiscV5(log, r, r.IP, r.UdpPort, r.PrivKey, bootNodes)
			if err != nil {
				log.Error(err)
				return
			}
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
			state.Dv5Node.Close()
			state.Dv5Node = nil
			log.Info("Stopped discv5")
		},
	})

	printLookupResult := func(nodes []*enode.Node) {
		enrs := make([]string, 0, len(nodes))
		for _, v := range nodes {
			enrs = append(enrs, v.String())
		}
		log.WithField("nodes", enrs).Infof("Lookup complete")
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "ping <target node: enode address or ENR (url-base64)>",
		Short: "Run discv5-ping",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			target, err := addrutil.ParseEnrOrEnode(args[0])
			if err != nil {
				log.Error(err)
			}
			if err := state.Dv5Node.Ping(target); err != nil {
				log.Errorf("Failed to ping: %v", err)
				return
			}
			log.Infof("Successfully pinged")
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
			target, err := addrutil.ParseEnrOrEnode(args[0])
			if err != nil {
				log.Error(err)
			}
			resolved := state.Dv5Node.Resolve(target)
			if resolved != nil {
				log.Errorf("Failed to resolve %s, nil result", target.String())
				return
			}
			log.WithField("enr", resolved.String()).Infof("Successfully resolved")
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
			target, err := addrutil.ParseEnrOrEnode(args[0])
			if err != nil {
				log.Error(err)
			}
			enrRes, err := state.Dv5Node.RequestENR(target)
			if err != nil {
				log.Error(err)
				return
			}
			log.WithField("enr", enrRes.String()).Infof("Successfully got ENR for node")
		},
	})

	lookupCmd := &cobra.Command{
		Use:   "lookup [target node: hex node ID, enode address or ENR (url-base64)]",
		Short: "Get list of nearby nodes. If no target node is provided, then find nodes nearby to self.",
		Args:  cobra.RangeArgs(0, 1),
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			target := state.Dv5Node.Self().ID()
			if len(args) > 0 {
				if n, err := addrutil.ParseEnrOrEnode(args[0]); err != nil {
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
			printLookupResult(state.Dv5Node.Lookup(target))
		},
	}
	cmd.AddCommand(lookupCmd)

	randomCommand := &cobra.Command{
		Use:   "lookup-random",
		Short: "Get list of random multi addrs.",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			printLookupResult(state.Dv5Node.LookupRandom())
		},
	}
	cmd.AddCommand(randomCommand)

	cmd.AddCommand(&cobra.Command{
		Use:   "self",
		Short: "get local discv5 ENR",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if noDv5(cmd) {
				return
			}
			log.WithField("enr", state.Dv5Node.Self()).Infof("local dv5 node")
		},
	})
	return cmd
}
