package custom

import (
	"fmt"
	csms "github.com/libp2p/go-conn-security-multistream"
	"github.com/libp2p/go-libp2p-core/connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/mux"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/pnet"
	"github.com/libp2p/go-libp2p-core/sec"
	"github.com/libp2p/go-libp2p-core/sec/insecure"
	"github.com/libp2p/go-libp2p-core/transport"
	tptu "github.com/libp2p/go-libp2p-transport-upgrader"
	"github.com/libp2p/go-libp2p/config"
	msmux "github.com/libp2p/go-stream-muxer-multistream"
)

type TransportOpts struct {
	Transports         []config.TptC
	Muxers             []config.MsMuxC
	SecurityTransports []config.MsSecC
	Insecure           bool
	PSK                pnet.PSK
}

func AddTransports(h host.Host, swrm transport.TransportNetwork, nodeOpts *NodeOpts, opts *TransportOpts) (err error) {
	upgrader := new(tptu.Upgrader)
	upgrader.PSK = opts.PSK
	upgrader.ConnGater = nodeOpts.ConnectionGater
	if opts.Insecure {
		upgrader.Secure = makeInsecureTransport(h.ID(), nodeOpts.PeerKey)
	} else {
		upgrader.Secure, err = makeSecurityTransport(h, opts.SecurityTransports)
		if err != nil {
			return err
		}
	}

	upgrader.Muxer, err = makeMuxer(h, opts.Muxers)
	if err != nil {
		return err
	}

	tpts, err := makeTransports(h, upgrader, nodeOpts.ConnectionGater, opts.Transports)
	if err != nil {
		return err
	}
	for _, t := range tpts {
		err = swrm.AddTransport(t)
		if err != nil {
			return err
		}
	}

	return nil
}

func makeMuxer(h host.Host, tpts []config.MsMuxC) (mux.Multiplexer, error) {
	muxMuxer := msmux.NewBlankTransport()
	transportSet := make(map[string]struct{}, len(tpts))
	for _, tptC := range tpts {
		if _, ok := transportSet[tptC.ID]; ok {
			return nil, fmt.Errorf("duplicate muxer transport: %s", tptC.ID)
		}
		transportSet[tptC.ID] = struct{}{}
	}
	for _, tptC := range tpts {
		tpt, err := tptC.MuxC(h)
		if err != nil {
			return nil, err
		}
		muxMuxer.AddTransport(tptC.ID, tpt)
	}
	return muxMuxer, nil
}

func makeTransports(h host.Host, u *tptu.Upgrader, cg connmgr.ConnectionGater, tpts []config.TptC) ([]transport.Transport, error) {
	transports := make([]transport.Transport, len(tpts))
	for i, tC := range tpts {
		t, err := tC(h, u, cg)
		if err != nil {
			return nil, err
		}
		transports[i] = t
	}
	return transports, nil
}

func makeInsecureTransport(id peer.ID, privKey crypto.PrivKey) sec.SecureTransport {
	secMuxer := new(csms.SSMuxer)
	secMuxer.AddTransport(insecure.ID, insecure.NewWithIdentity(id, privKey))
	return secMuxer
}

func makeSecurityTransport(h host.Host, tpts []config.MsSecC) (sec.SecureTransport, error) {
	secMuxer := new(csms.SSMuxer)
	transportSet := make(map[string]struct{}, len(tpts))
	for _, tptC := range tpts {
		if _, ok := transportSet[tptC.ID]; ok {
			return nil, fmt.Errorf("duplicate security transport: %s", tptC.ID)
		}
		transportSet[tptC.ID] = struct{}{}
	}
	for _, tptC := range tpts {
		tpt, err := tptC.SecC(h)
		if err != nil {
			return nil, err
		}
		if _, ok := tpt.(*insecure.Transport); ok {
			return nil, fmt.Errorf("cannot construct libp2p with an insecure transport, set the Insecure config option instead")
		}
		secMuxer.AddTransport(tptC.ID, tpt)
	}
	return secMuxer, nil
}
