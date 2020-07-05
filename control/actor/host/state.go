package host

import (
	"errors"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"sync"
)

type HostState struct {
	// only one routine can modify the host at a time
	hLock   sync.Mutex
	p2pHost host.Host
}

// shortcut to check if there is a libp2p host available, and error-log if not available.
func (s *HostState) Host() (h host.Host, err error) {
	h = s.p2pHost
	if h == nil {
		return nil, errors.New("Must first initialize Libp2p host. Try 'host start'")
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
	return nil
}

func (s *HostState) GetHostPriv() *crypto.Secp256k1PrivateKey {
	s.hLock.Lock()
	defer s.hLock.Unlock()
	if s.p2pHost == nil {
		return nil
	}
	priv := s.p2pHost.Peerstore().PrivKey(s.p2pHost.ID())
	if priv == nil {
		return nil
	}
	return priv.(*crypto.Secp256k1PrivateKey)
}
