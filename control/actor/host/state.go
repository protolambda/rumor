package host

import (
	"crypto/ecdsa"
	"errors"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/protolambda/rumor/p2p/addrutil"
	"net"
	"sync"
)

type HostState struct {
	// only one routine can modify the host at a time
	hLock   sync.Mutex
	p2pHost host.Host

	PrivKey crypto.PrivKey

	IP      net.IP
	TcpPort uint16
	UdpPort uint16
}

// shortcut to check if there is a libp2p host available, and error-log if not available.
func (s *HostState) Host() (h host.Host, err error) {
	h = s.p2pHost
	if h == nil {
		return nil, errors.New("REPL must have initialized Libp2p host. Try 'host start'")
	}
	return h, nil
}

func (s *HostState) SetHost(h host.Host) error {
	s.hLock.Lock()
	defer s.hLock.Unlock()
	if s.p2pHost != nil {
		return errors.New("existing host, cannot change host before closing old host")
	}
	s.p2pHost = h
	return nil
}

func (s *HostState) CloseHost() error {
	s.hLock.Lock()
	defer s.hLock.Unlock()
	if s.p2pHost == nil {
		return errors.New("no host was open")
	}
	err := s.p2pHost.Close()
	if err != nil {
		return err
	}
	s.p2pHost = nil
}

func (s *HostState) GetEnr() *enr.Record {
	priv := (*ecdsa.PrivateKey)(s.PrivKey.(*crypto.Secp256k1PrivateKey))
	return addrutil.MakeENR(s.IP, s.TcpPort, s.UdpPort, priv)
}
