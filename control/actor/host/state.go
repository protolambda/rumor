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

	IP      net.IP
	TcpPort uint16
	UdpPort uint16
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

func (s *HostState) GetEnr() *enr.Record {
	privIfc := s.GetPriv()
	if privIfc == nil {
		return addrutil.MakeENR(s.IP, s.TcpPort, s.UdpPort, nil)
	}
	priv := (*ecdsa.PrivateKey)(privIfc.(*crypto.Secp256k1PrivateKey))
	return addrutil.MakeENR(s.IP, s.TcpPort, s.UdpPort, priv)
}

func (s *HostState) GetPriv() crypto.PrivKey {
	s.hLock.Lock()
	defer s.hLock.Unlock()
	return s.p2pHost.Peerstore().PrivKey(s.p2pHost.ID())
}

// TODO: can we make these changes effective without restarting the host?

func (s *HostState) GetIP() net.IP {
	return s.IP
}

func (s *HostState) SetIP(ip net.IP) {
	s.IP = ip
}

func (s *HostState) GetTCP() uint16 {
	return s.TcpPort
}

func (s *HostState) SetTCP(port uint16) {
	s.TcpPort = port
}

func (s *HostState) GetUDP() uint16 {
	return s.UdpPort
}

func (s *HostState) SetUDP(port uint16) {
	s.UdpPort = port
}
