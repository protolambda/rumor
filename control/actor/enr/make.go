package enr

import (
	"context"
	"crypto/ecdsa"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	gcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/libp2p/go-libp2p-core/crypto"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/rumor/p2p/peering/enrstate"
	"github.com/protolambda/zrnt/eth2/beacon"
	"net"
	"strconv"
)

type EnrMakeCmd struct {
	*base.Base
	Lazy *LazyEnrState
	base.WithHostPriv
	base.PrivSettings

	FromHost bool `ask:"--from-host" help:"If IP, TCP and UDP should copy the libp2p host settings, if not present in the ENR already"`

	RmIP        bool   `ask:"--rm-ip"`
	IP          net.IP `ask:"--ip" help:"IP address dv4. Currrent ENR IP if empty."`
	StaticIP    net.IP `ask:"--static-ip" help:"Set a static IP in the ENR."`
	FallbackIP  net.IP `ask:"--fallback-ip" help:"Set a fallback IP, used when no public IP is known, until it can be determined from traffic."`
	FallbackUDP uint16 `ask:"--fallback-udp" help:"Set a fallback UDP port, used when no public port is known, until it can be determined from traffic."`
	TCP         uint16 `ask:"--tcp" help:"TCP port. Current ENR port if 0"`
	UDP         uint16 `ask:"--udp" help:"UDP port. Current ENR port if 0"`

	GenPriv bool                 `ask:"--gen-priv" help:"If no private key is known, and none is provided, then generate one"`
	Priv    flags.P2pPrivKeyFlag `ask:"--priv" help:"Private key, in raw hex encoded format (32 bytes -> 64 hex chars with optional 0x prefix). Cannot overwrite a current private key."`

	Attnets         beacon.AttnetBits `ask:"--attnets" help:"Attnet bitfield, as bytes."`
	ForkDigest      beacon.ForkDigest `ask:"--fork-digest" help:"Eth2 fork digest"`
	NextForkVersion beacon.Version    `ask:"--next-fork-version" help:"Eth2 next fork version"`
	NextForkEpoch   beacon.Epoch      `ask:"--next-fork-epoch" help:"Eth2 next fork epoch"`

	IPChanged          bool `changed:"ip"`
	StaticIPChanged    bool `changed:"static-ip"`
	FallbackIPChanged  bool `changed:"fallback-ip"`
	FallbackUDPChanged bool `changed:"fallback-udp"`
	TCPChanged         bool `changed:"tcp"`
	UDPChanged         bool `changed:"udp"`

	AttnetsChanged         bool `changed:"attnets"`
	ForkDigestChanged      bool `changed:"fork-digest"`
	NextForkVersionChanged bool `changed:"next-fork-version"`
	NextForkEpochChanged   bool `changed:"next-fork-epoch"`
}

func (c *EnrMakeCmd) Default() {
	c.GenPriv = true
	c.FromHost = true
	c.StaticIP = nil
	c.FallbackIP = nil
	c.FallbackUDP = 0
	c.IP = nil
	c.TCP = 9000
	c.UDP = 9000
}

func (c *EnrMakeCmd) Help() string {
	return "Make an ENR, incl. random private key if necessary. Merges into existing ENR settings (if any)."
}

func (c *EnrMakeCmd) Run(ctx context.Context, args ...string) error {
	priv := c.Priv.Priv
	if priv == nil && c.Lazy.Current != nil {
		priv = c.Lazy.Current.GetPriv()
	}

	if priv == nil {
		// try get the current non-ENR private key, may be nil
		priv = c.GetPriv()
	}

	if priv == nil {
		if c.GenPriv {
			key, err := ecdsa.GenerateKey(gcrypto.S256(), crand.Reader)
			if err != nil {
				return fmt.Errorf("failed to generate key: %v", err)
			}
			priv = (*crypto.Secp256k1PrivateKey)(key)
			keyBytes, err := priv.Raw()
			if err != nil {
				return fmt.Errorf("failed to serialize key: %v", err)
			}
			c.Log.WithField("key", hex.EncodeToString(keyBytes)).Infoln("generated key")
		} else {
			return fmt.Errorf("no private key known, but key-gen disabled, cannot make ENR")
		}
	}

	// once locked in, it can never change for the actor.
	// Try writing it, if it errors there is a different key already in use (catches race condition of two 'enr make')
	if err := c.SetPriv(priv); err != nil {
		return fmt.Errorf("private key conflict: %v", err)
	}

	if c.Lazy.Current == nil {
		state, err := enrstate.NewEnrState(c.StaticIP, c.FallbackIP, c.FallbackUDP, priv)
		if err != nil {
			return fmt.Errorf("could not make local ENR state: %v", err)
		}
		c.Lazy.Current = state
	}

	// If we can copy settings from the p2p host, and we have unset options, then try retrieve values from the host.
	if c.FromHost && (!c.IPChanged || !c.TCPChanged || !c.UDPChanged) {
		// If there is an existing libp2p host, it may already have made a port choice for us to default to.
		// Or better, a public IP, derived with something like upnp or pmp
		if h, err := c.Host(); err == nil {
			var hostIp net.IP = nil
			var hostTCP uint16 = 0
			// The last address is generally the most public, keep looping.
			for _, a := range h.Addrs() {
				if hostTCP == 0 {
					if addrTCP, err := a.ValueForProtocol(ma.P_TCP); err == nil {
						v, err := strconv.ParseUint(addrTCP, 10, 16)
						if err == nil {
							hostTCP = uint16(v)
						}
					}
				}
				if addrIp, err := a.ValueForProtocol(ma.P_IP4); err == nil {
					hostIp = net.ParseIP(addrIp)
				}
			}
			if !c.IPChanged && hostIp != nil {
				c.IPChanged = true
				c.IP = hostIp
			}
			if !c.TCPChanged && hostTCP != 0 {
				c.TCPChanged = true
				c.TCP = hostTCP
			}
			if !c.UDPChanged && hostTCP != 0 {
				// just copy TCP if we've a tcp port
				c.UDPChanged = true
				c.UDP = hostTCP
			}
		}
	}

	node := c.Lazy.Current.GetNode()

	if c.IPChanged || node.IP() == nil {
		c.Lazy.Current.SetIP(c.IP)
	}

	if c.TCPChanged || node.TCP() == 0 {
		c.Lazy.Current.SetTCP(c.TCP)
	}

	if c.UDPChanged || node.UDP() == 0 {
		c.Lazy.Current.SetUDP(c.UDP)
	}

	if c.AttnetsChanged {
		c.Lazy.Current.SetAttnets(&c.Attnets)
	}

	if c.ForkDigestChanged || c.NextForkVersionChanged || c.NextForkEpochChanged {
		// If they didn't all change, merge in the existing data (if any)
		if !(c.ForkDigestChanged && c.NextForkVersionChanged && c.NextForkEpochChanged) {
			var currEth2 *beacon.Eth2Data = nil
			if dat, exists, err := addrutil.ParseEnrEth2Data(node); err != nil && exists {
				currEth2 = dat
			}

			if currEth2 != nil {
				if !c.ForkDigestChanged {
					c.ForkDigest = currEth2.ForkDigest
				}
				if !c.NextForkVersionChanged {
					c.NextForkVersion = currEth2.NextForkVersion
				}
				if !c.NextForkEpochChanged {
					c.NextForkEpoch = currEth2.NextForkEpoch
				}
			}
		}

		c.Lazy.Current.SetEth2Data(&beacon.Eth2Data{
			ForkDigest:      c.ForkDigest,
			NextForkVersion: c.NextForkVersion,
			NextForkEpoch:   c.NextForkEpoch,
		})
	}

	if c.RmIP {
		c.Lazy.Current.SetIP(nil)
	}

	node = c.Lazy.Current.GetNode()
	c.Log.WithField("enr", node.String()).Infof("ENR created!")
	return nil
}
