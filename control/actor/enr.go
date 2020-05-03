package actor

import (
	"context"
	"crypto/ecdsa"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/btcec"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"net"
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
				"seq":     enodeRes.Seq(),
				"xy":      fmt.Sprintf("%d %d", pubkey.X, pubkey.Y),
				"node_id": nodeID.String(),
				"peer_id": peerID.String(),
				"enode":   enodeRes.URLv4(),
				"multi":   muAddr.String(),
				"enr":     enodeRes.String(), // formats as url-base64 ENR
			}).Info("ENR parsed successfully")
		},
	}
	enrViewCmd.Flags().BoolVar(&parseEnrKV, "kv", false, "Print the full set of Key-Value pairs")
	cmd.AddCommand(enrViewCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "gen-key",
		Short: "Make a private key for an ENR.",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			key, err := ecdsa.GenerateKey(btcec.S256(), crand.Reader)
			if err != nil {
				log.Errorf("failed to generate key: %v", err)
			}
			secpKey := (*crypto.Secp256k1PrivateKey)(key)
			keyBytes, err := secpKey.Raw()
			if err != nil {
				log.Errorf("failed to serialize key: %v", err)
			}
			log.WithField("key", hex.EncodeToString(keyBytes)).Infoln("generated key")
		},
	})

	{
		var ipStr string
		var tcpPort, udpPort uint16
		var privStr string
		buildCmd := &cobra.Command{
			Use:   "make",
			Short: "Make an ENR. ENR is url-base64 (RFC 4648).",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				var ip net.IP
				if ipStr != "" {
					ip = net.ParseIP(ipStr)
					if ip == nil {
						log.Errorf("could not parse ip: %s", ipStr)
						return
					}
				}
				var priv *ecdsa.PrivateKey
				if privStr != "" {
					var err error
					priv, err = addrutil.ParsePrivateKey(privStr)
					if err != nil {
						log.Errorf("could not parse private key: %v", err)
						return
					}
				}
				rec := addrutil.MakeENR(ip, tcpPort, udpPort, priv)
				enrStr, err := addrutil.EnrToString(rec)
				if err != nil {
					log.Error(err)
					return
				}
				log.WithField("enr", enrStr).Infof("Enr address")
			},
		}
		// TODO: add an "eth2" key option
		buildCmd.Flags().StringVar(&ipStr, "ip", "", "IP address (v4 or v6). None if empty.")
		buildCmd.Flags().Uint16Var(&tcpPort, "tcp", 0, "TCP port. None if 0")
		buildCmd.Flags().Uint16Var(&udpPort, "udp", 0, "UDP port. None if 0")
		buildCmd.Flags().StringVar(&privStr, "priv", "", "Private key, in raw hex encoded format (32 bytes -> 64 hex chars with optional 0x prefix). None if empty, and also no signed ENR.")
		cmd.AddCommand(buildCmd)
	}
	return cmd
}
