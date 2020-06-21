package chain

import (
	"github.com/protolambda/rumor/chain"
)

type ChainState struct {
	CurrentChain chain.ChainID
}

/*
func InitChainCmd(ctx context.Context, log logrus.FieldLogger) {
	//cmd := &cobra.Command{
	//	Use:   "chain",
	//	Short: "Manage Eth2 chains, forkchoice too",
	//}

	// TODO refactor old chain commands


	cmd.AddCommand(&cobra.Command{
		Use:   "switch <chain-name>",
		Short: "Switch to an existing chain",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			r.ChainState.CurrentChain = chain.ChainID(args[0])
		},
	})
	{
		chainCreateCmd := &cobra.Command{
			Use:   "create",
			Short: "Create a new chain",
		}

		createChain := func(chainId chain.ChainID, state *beacon.BeaconStateView) error {
			slot, err := state.Slot()
			if err != nil {
				return err
			}
			latestHeader, err := state.LatestBlockHeader()
			if err != nil {
				return err
			}
			latestHeader, err = beacon.AsBeaconBlockHeader(latestHeader.Copy())
			if err != nil {
				return err
			}
			if err := latestHeader.SetStateRoot(state.HashTreeRoot(tree.GetHashFn())); err != nil {
				return err
			}
			blockRoot := latestHeader.HashTreeRoot(tree.GetHashFn())
			parentRoot, err := latestHeader.ParentRoot()
			if err != nil {
				return err
			}
			epc, err := state.NewEpochsContext()
			if err != nil {
				return err
			}
			entry := chain.NewHotEntry(slot, blockRoot, parentRoot, state, epc)
			_, err = r.GlobalChains.Create(chainId, entry)
			return err
		}
		chainCreateCmd.AddCommand(&cobra.Command{
			Use:   "hex <chain-name> <state encoded in hex>",
			Short: "Create chain by starting from given state",
			Args:  cobra.ExactArgs(2),
			Run: func(cmd *cobra.Command, args []string) {
				name := chain.ChainID(args[0])
				r.ChainState.CurrentChain = name
				var state *beacon.BeaconStateView
				{
					bytez, err := hex.DecodeString(args[1])
					if err != nil {
						log.Error(err)
						return
					}
					state, err = beacon.AsBeaconStateView(
						beacon.BeaconStateType.Deserialize(bytes.NewReader(bytez), uint64(len(bytez))))
					if err != nil {
						log.Error(err)
						return
					}
					log.Infoln("loaded starting state state")
				}
				if err := createChain(name, state); err != nil {
					log.Error(err)
					return
				}
			},
		})
		chainCreateCmd.AddCommand(&cobra.Command{
			Use:   "file <chain-name> <state ssz file>",
			Short: "Create chain by starting from given state",
			Args:  cobra.ExactArgs(2),
			Run: func(cmd *cobra.Command, args []string) {
				name := chain.ChainID(args[0])
				r.ChainState.CurrentChain = name
				var state *beacon.BeaconStateView
				{
					stateFile := args[0]
					fSt, err := os.Stat(stateFile)
					if err != nil {
						log.Error(err)
						return
					}
					f, err := os.Open(stateFile)
					if err != nil {
						log.Error(err)
						return
					}
					defer f.Close()
					state, err = beacon.AsBeaconStateView(beacon.BeaconStateType.Deserialize(f, uint64(fSt.Size())))
					if err != nil {
						log.Error(err)
						return
					}
					log.Infoln("loaded starting state state")
				}
				if err := createChain(name, state); err != nil {
					log.Error(err)
					return
				}
			},
		})
		cmd.AddCommand(chainCreateCmd)
	}
	// TODO chain copy <chain-name>  # fork the chain by creating a copy of it
	{
		chainHotCmd := &cobra.Command{
			Use:   "hot",
			Short: "Manage the hot chain",
		}
		chainHotCmd.AddCommand(&cobra.Command{
			Use:   "view",
			Short: "View the hot chain contents",
			Args:  cobra.ExactArgs(2),
			Run: func(cmd *cobra.Command, args []string) {
				name := chain.ChainID(args[0])
				c, ok := r.GlobalChains.Find(name)
				if !ok {
					log.Errorf("chain '%s' does not exist", name)
				}
				entries := make([]map[string]interface{}, 0)

				iter := c.HotChain.Iter()
				for {
					entry, ok, err := iter.ThisEntry()
					if err != nil {
						log.Error(err)
					}
					if !ok {
						break
					}

					entries = append(entries, map[string]interface{}{
						"slot":        entry.Slot(),
						"block_root":  entry.BlockRoot(),
						"state_root":  entry.StateRoot(),
						"parent_root": entry.ParentRoot(),
						"empty":       entry.IsEmpty(),
					})
					iter.NextSlot()
				}
				log.WithField("entries", entries).Infof("Hot chain has %d entries", len(entries))
			},
		})
	}
	// TODO chain cold view <start> <stop>

	return cmd
}
 */
