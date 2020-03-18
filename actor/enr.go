package actor

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/protolambda/rumor/addrutil"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"net"
	"strconv"
)

func (r *Actor) InitEnrCmd(ctx context.Context, log logrus.FieldLogger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enr",
		Short: "Ethereum Name Record (ENR) utilities",
	}
	var parseEnrKV bool
	enrViewCmd := &cobra.Command{
		Use:   "view <enr>",
		Short: "view ENR contents. ENR is url-base64 (RFC 4648). With optional 'enr:' or 'enr://' prefix.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			enrStr := args[0]
			rec, err := addrutil.ParseEnr(enrStr)
			if err != nil {
				log.Error(err)
				return
			}
			if parseEnrKV {
				enrPairs := rec.AppendElements(nil)
				enrKV := make(map[string]string)
				enrKVraw := make(map[string]string)
				for i := 1; i < len(enrPairs); i += 2 {
					key := enrPairs[i].(string)
					rawValue := enrPairs[i+1].(rlp.RawValue)

					enrKVraw[key] = hex.EncodeToString(rawValue)
					getTypedValue, ok := addrutil.EnrEntries[key]
					if !ok {
						log.WithField("enr_unknown_"+key, rawValue).Infof("Unrecognized ENR KV pair type: %s", key)
					} else {
						typedValue, getValueStr := getTypedValue()
						if err := rlp.DecodeBytes(rawValue, typedValue); err != nil {
							log.WithField("enr_fail_"+key, rawValue).Errorf("Failed to decode ENR KV pair: %s", key)
						}
						enrKV[key] = getValueStr()
					}
				}

				log.WithField("raw_enr_kv", enrKVraw).Info("Raw ENR key-value pairs")
				log.WithField("enr_kv", enrKV).Info("Decoded ENR key-value pairs")
			}

			var enodeRes *enode.Node
			enodeRes, err = addrutil.EnrToEnode(rec, true)
			if err != nil {
				log.Error(err)
				return
			}

			pubkey := enodeRes.Pubkey()
			peerID := addrutil.PeerIDFromPubkey(pubkey)
			nodeID := enode.PubkeyToIDV4(pubkey)
			muAddr, err := addrutil.EnodeToMultiAddr(enodeRes)
			if err != nil {
				log.Error(err)
				return
			}
			log.WithFields(logrus.Fields{
				"seq": enodeRes.Seq(),
				"xy": fmt.Sprintf("%d %d", pubkey.X, pubkey.Y),
				"node_id": nodeID.String(),
				"peer_id": peerID.String(),
				"enode": enodeRes.URLv4(),
				"multi": muAddr.String(),
				"enr": enodeRes.String(),  // formats as url-base64 ENR
			}).Info("ENR parsed successfully")
		},
	}
	enrViewCmd.Flags().BoolVar(&parseEnrKV, "kv", false, "Print the full set of Key-Value pairs")
	cmd.AddCommand(enrViewCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "make <ip> <tcp-port> <udp-port> <priv>",
		Short: "make an ENR. ENR is url-base64 (RFC 4648). Pubkey is raw hex encoded format",
		Args:  cobra.ExactArgs(4),
		Run: func(cmd *cobra.Command, args []string) {
			ip := net.ParseIP(args[0])
			if ip == nil {
				log.Errorf("could not parse ip: %s", args[0])
				return
			}
			tcpPort, err := strconv.ParseUint(args[1], 0, 16)
			if err != nil {
				log.Errorf("could not parse tcp port: %v", err)
				return
			}
			udpPort, err := strconv.ParseUint(args[2], 0, 16)
			if err != nil {
				log.Errorf("could not parse udp port: %v", err)
				return
			}
			priv, err := addrutil.ParsePrivateKey(args[3])
			if err != nil {
				log.Errorf("could not pubkey: %v", err)
				return
			}
			rec := addrutil.MakeENR(ip, uint16(tcpPort), uint16(udpPort), priv)
			enrStr, err := addrutil.EnrToString(rec)
			if err != nil {
				log.Error(err)
				return
			}
			log.Infof("ENR address: %s", enrStr)
		},
	})
	return cmd
}
