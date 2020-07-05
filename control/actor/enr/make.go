package enr

import (
	"context"
	"crypto/ecdsa"
	crand "crypto/rand"
	"fmt"
	"github.com/btcsuite/btcd/btcec"
	"github.com/libp2p/go-libp2p-core/crypto"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/p2p/peering/enrstate"
	"net"
	"strconv"
)

type EnrMakeCmd struct {
	*base.Base
	Lazy *LazyEnrState
	base.WithHostPriv
	base.PrivSettings

	MergeCurrent bool                 `ask:"--current" help:"If the current settings should be used as default for missing options"`
	IP           net.IP               `ask:"--ip" help:"IP address dv4. Currrent ENR IP if empty."`
	StaticIP     net.IP               `ask:"--static-ip" help:"Set a static IP in the ENR."`
	FallbackIP   net.IP               `ask:"--fallback-ip" help:"Set a fallback IP, used when no public IP is known, until it can be determined from traffic."`
	FallbackUDP  uint16               `ask:"--fallback-udp" help:"Set a fallback UDP port, used when no public port is known, until it can be determined from traffic."`
	TCP          uint16               `ask:"--tcp" help:"TCP port. Current ENR port if 0"`
	UDP          uint16               `ask:"--udp" help:"UDP port. Current ENR port if 0"`
	GenPriv      bool                 `ask:"--gen-priv" help:"If no private key is known, and none is provided, then generate one"`
	Priv         flags.P2pPrivKeyFlag `ask:"--priv" help:"Private key, in raw hex encoded format (32 bytes -> 64 hex chars with optional 0x prefix). Cannot overwrite a current private key."`
}

func (c *EnrMakeCmd) Default() {
	c.GenPriv = true
	c.MergeCurrent = true
}

func (c *EnrMakeCmd) Help() string {
	return "Make a private key for an ENR. Optionally override current ENR settings."
}

func (c *EnrMakeCmd) Run(ctx context.Context, args ...string) error {
	ip := c.IP
	tcp := c.TCP
	udp := c.UDP
	priv := c.Priv.Priv
	var currIp net.IP = nil
	var currTCP uint16 = 0
	var currUDP uint16 = 0
	var currPriv *crypto.Secp256k1PrivateKey = nil
	if c.MergeCurrent && c.Lazy.Current != nil {
		node := c.Lazy.Current.GetNode()
		currIp = node.IP()
		currTCP = uint16(node.TCP())
		currUDP = uint16(node.UDP())
		currPriv = c.Lazy.Current.GetPriv()
		if ip == nil {
			ip = currIp
		}
		if tcp == 0 {
			tcp = currTCP
		}
		if udp == 0 {
			udp = currUDP
		}
		if priv == nil {
			priv = currPriv
		}
	}
	// If there is an existing libp2p host, it may already have made a port choice for us to default to.
	// Or better, a public IP, derived with something like upnp or pmp
	if h, err := c.Host(); err == nil {
		var existingIp net.IP = nil
		var existingTCP uint16 = 0
		// The last address is generally the most public, keep looping.
		for _, a := range h.Addrs() {
			if existingTCP == 0 {
				if addrTCP, err := a.ValueForProtocol(ma.P_TCP); err == nil {
					v, err := strconv.ParseUint(addrTCP, 10, 16)
					if err == nil {
						existingTCP = uint16(v)
					}
				}
			}
			if addrIp, err := a.ValueForProtocol(ma.P_IP4); err == nil {
				existingIp = net.ParseIP(addrIp)
			}
		}
		if ip == nil {
			ip = existingIp
		}
		if tcp == 0 {
			tcp = existingTCP
		}
		if udp == 0 {
			udp = existingTCP // just copy TCP if we've a tcp port
		}
	}
	if tcp == 0 {
		tcp = 9000
	}
	if udp == 0 {
		udp = 9000
	}

	if ip == nil {
		ip = net.IPv4zero
	}
	if priv == nil {
		// try get the current non-ENR private key, may be nil
		priv = c.GetPriv()
	}

	if priv == nil {
		if c.GenPriv {
			key, err := ecdsa.GenerateKey(btcec.S256(), crand.Reader)
			if err != nil {
				return fmt.Errorf("failed to generate key: %v", err)
			}
			priv = (*crypto.Secp256k1PrivateKey)(key)
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

	// avoid unnecessary seq updates by checking if data matches

	if currIp == nil || !currIp.Equal(ip) {
		c.Lazy.Current.SetIP(ip)
	}
	if currUDP != udp {
		c.Lazy.Current.SetUDP(udp)
	}
	if currTCP != tcp {
		c.Lazy.Current.SetTCP(tcp)
	}

	node := c.Lazy.Current.GetNode()

	c.Log.WithField("enr", node.String()).Infof("ENR created!")
	return nil
}
