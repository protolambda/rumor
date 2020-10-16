package host

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	gcrypto "github.com/ethereum/go-ethereum/crypto"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	mplex "github.com/libp2p/go-libp2p-mplex"
	noise "github.com/libp2p/go-libp2p-noise"
	secio "github.com/libp2p/go-libp2p-secio"
	tls "github.com/libp2p/go-libp2p-tls"
	yamux "github.com/libp2p/go-libp2p-yamux"
	"github.com/libp2p/go-libp2p/config"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	"github.com/libp2p/go-tcp-transport"
	ws "github.com/libp2p/go-ws-transport"
	"github.com/protolambda/rumor/control/actor/base"
	"github.com/protolambda/rumor/control/actor/flags"
	"github.com/protolambda/rumor/control/actor/peerstore"
	"github.com/protolambda/rumor/p2p/custom"
	"github.com/protolambda/rumor/p2p/track"
	"strings"
	"time"
)

type HostStartCmd struct {
	*base.Base
	base.PrivSettings
	WithSetHost

	GlobalPeerstores track.Peerstores
	CurrentPeerstore track.DynamicPeerstore

	PrivKey            flags.P2pPrivKeyFlag `ask:"--priv" help:"hex-encoded private key for libp2p host. Random if none is specified."`
	TransportsStrArr   []string             `ask:"--transport" help:"Transports to use. Options: tcp, ws"`
	MuxStrArr          []string             `ask:"--mux" help:"Multiplexers to use"`
	SecurityArr        []string             `ask:"--security" help:"Security to use. Multiple can be selected, order matters. Options: secio, noise, tls, none"`
	RelayEnabled       bool                 `ask:"--relay" help:"enable relayer functionality"`
	LoPeers            int                  `ask:"--lo-peers" help:"low-water for connection manager to trim peer count to"`
	HiPeers            int                  `ask:"--hi-peers" help:"high-water for connection manager to trim peer count from"`
	GracePeriod        time.Duration        `ask:"--peer-grace-period" help:"Time to grace a peer from being trimmed"`
	NatEnabled         bool                 `ask:"--nat" help:"enable nat address discovery (upnp/pmp)"`
	UserAgent          string               `ask:"--agent" help:"user agent string to use in libp2p identify protocol"`
	EnableIdentify     bool                 `ask:"--identify" help:"Enable the libp2p identify protocol"`
	IDFirst            bool                 `ask:"--identify-first" help:"Try and identify upon connecting with the host"`
	EnablePing         bool                 `ask:"--libp2p-ping" help:"Enable the libp2p ping background service"`
	NegotiationTimeout time.Duration        `ask:"--negotiation-timeout" help:"Time to allow for negotiation. Negative to disable."`
	SignedPeerRecord   bool                 `ask:"--signed-peer-records" help:"Use signed peer records"`
}

func (c *HostStartCmd) Default() {
	c.TransportsStrArr = []string{"tcp"}
	c.MuxStrArr = []string{"yamux", "mplex"}
	c.SecurityArr = []string{"noise", "secio"}
	c.LoPeers = 15
	c.HiPeers = 20
	c.GracePeriod = 20 * time.Second
	c.NatEnabled = true
	c.UserAgent = "Rumor"
	c.EnableIdentify = true
	c.IDFirst = true
	c.EnablePing = false
	c.NegotiationTimeout = custom.DefaultNegotiationTimeout
	c.SignedPeerRecord = false
}

func (c *HostStartCmd) Help() string {
	return "Start the host node. See flags for security, transport, mux etc. options"
}

func (c *HostStartCmd) Run(ctx context.Context, args ...string) error {
	_, err := c.Host()
	if err == nil {
		return errors.New("already have a host open")
	}

	hostOptions := custom.Config{}
	{
		var priv *crypto.Secp256k1PrivateKey
		if c.PrivKey.Priv == nil {
			// Check if we already have a key
			if current := c.GetPriv(); current == nil {
				// generate new private key if non was specified
				genp, err := ecdsa.GenerateKey(gcrypto.S256(), rand.Reader)
				if err != nil {
					return fmt.Errorf("failed to generate key: %v", err)
				}
				priv = (*crypto.Secp256k1PrivateKey)(genp)
				priv.Curve = gcrypto.S256()
				p, err := priv.Raw()
				if err != nil {
					return err
				}
				// handles race conditions, if we need to, stick with the existing key.
				if err := c.PrivSettings.SetPriv(priv); err == nil {
					c.Log.WithField("priv", hex.EncodeToString(p)).Info("Generated random Secp256k1 private key")
				} else {
					priv = c.PrivSettings.GetPriv()
				}
			} else {
				priv = current
			}
		} else {
			// User started a host with explicit private key.
			// Check if we don't have one currently, or if it's the same anyway.
			priv = c.PrivKey.Priv
			if err := c.PrivSettings.SetPriv(priv); err != nil { // checks against existing key for us.
				return fmt.Errorf("cannot overwrite existing actor private key: %v", err)
			}
		}
		hostOptions.PeerKey = priv
	}

	for _, v := range c.TransportsStrArr {
		v = strings.ToLower(strings.TrimSpace(v))
		var tp interface{}
		switch v {
		case "tcp":
			tp = tcp.NewTCPTransport
		case "ws":
			tp = ws.New
		default:
			return fmt.Errorf("could not recognize transport %s", v)
		}
		tptc, err := config.TransportConstructor(tp)
		if err != nil {
			return err
		}
		hostOptions.Transports = append(hostOptions.Transports, tptc)
	}

	for _, v := range c.MuxStrArr {
		v = strings.ToLower(strings.TrimSpace(v))
		var mc config.MsMuxC
		switch v {
		case "yamux":
			mtpt, err := config.MuxerConstructor(yamux.DefaultTransport)
			if err != nil {
				return err
			}
			mc = config.MsMuxC{MuxC: mtpt, ID: "/yamux/1.0.0"}
		case "mplex":
			mtpt, err := config.MuxerConstructor(mplex.DefaultTransport)
			if err != nil {
				return err
			}
			mc = config.MsMuxC{MuxC: mtpt, ID: "/mplex/6.7.0"}
		default:
			return fmt.Errorf("could not recognize mux %s", v)
		}
		hostOptions.Muxers = append(hostOptions.Muxers, mc)
	}

	for _, secOpt := range c.SecurityArr {
		var sc config.MsSecC
		switch secOpt {
		case "none":
			// no security, for debugging etc.
			if len(hostOptions.SecurityTransports) > 0 || len(c.SecurityArr) > 1 {
				return errors.New("cannot mix secure transport protocols with no-security")
			}
			hostOptions.Insecure = true
		case "secio":
			stpt, err := config.SecurityConstructor(secio.New)
			if err != nil {
				return err
			}
			sc = config.MsSecC{SecC: stpt, ID: secio.ID}
		case "noise":
			stpt, err := config.SecurityConstructor(noise.New)
			if err != nil {
				return err
			}
			sc = config.MsSecC{SecC: stpt, ID: noise.ID}
		case "tls":
			stpt, err := config.SecurityConstructor(tls.New)
			if err != nil {
				return err
			}
			sc = config.MsSecC{SecC: stpt, ID: tls.ID}
		default:
			return fmt.Errorf("could not recognize security %s", secOpt)
		}
		hostOptions.SecurityTransports = append(hostOptions.SecurityTransports, sc)
	}

	if c.NatEnabled {
		hostOptions.NATManager = basichost.NewNATManager
	}

	store := c.CurrentPeerstore
	if !store.Initialized() {
		// run peerstore command to create peerstore, put it into global stores, and init current
		createStore := peerstore.CreateCmd{
			Base:             c.Base,
			GlobalPeerstores: c.GlobalPeerstores,
			CurrentPeerstore: c.CurrentPeerstore,
			Switch:           true,
		}
		if err := createStore.Run(ctx); err != nil {
			return fmt.Errorf("failed to create peerstore: %v", err)
		}
	}
	hostOptions.Peerstore = store
	hostOptions.ConnManager = connmgr.NewConnManager(c.LoPeers, c.HiPeers, c.GracePeriod)
	hostOptions.UserAgent = c.UserAgent
	hostOptions.EnablePing = c.EnablePing
	hostOptions.DisableSignedPeerRecord = !c.SignedPeerRecord
	hostOptions.IDFirst = c.IDFirst
	hostOptions.EnableIdentify = c.EnableIdentify
	// TODO: peer metrics data hostOptions.Reporter
	// TODO: hostOptions.ConnectionGater
	hostOptions.NegotiationTimeout = c.NegotiationTimeout
	// Not the command ctx, we want the host to stay open after the command.
	h, err := custom.NewNode(c.ActorContext, &hostOptions)
	if err != nil {
		return err
	}
	return c.SetHost(h)
}
