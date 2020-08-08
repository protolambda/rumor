package custom

import (
	"context"
	"github.com/libp2p/go-libp2p-core/connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
)

type starter interface {
	Start()
}

type NodeOpts struct {
	PeerKey         crypto.PrivKey
	ConnectionGater connmgr.ConnectionGater
}

type Config struct {
	NodeOpts
	HostOpts
	NetworkOpts
	TransportOpts
}

// NewNode constructs a new libp2p Host from the Config.
//
// This function consumes the config. Do not reuse it (really!).
func NewNode(ctx context.Context, cfg *Config) (host.Host, error) {
	swrm, err := NewNetwork(ctx, &cfg.NodeOpts, &cfg.NetworkOpts)
	if err != nil {
		return nil, err
	}

	var h host.Host
	h, err = NewHost(ctx, swrm, &cfg.HostOpts)

	if err != nil {
		swrm.Close()
		return nil, err
	}

	err = AddTransports(h, swrm, &cfg.NodeOpts, &cfg.TransportOpts)
	if err != nil {
		h.Close()
		return nil, err
	}

	// start the host background tasks
	if st, ok := h.(starter); ok {
		st.Start()
	}

	return h, nil
}
