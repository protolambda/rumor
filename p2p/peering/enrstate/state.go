package enrstate

import (
	"crypto/ecdsa"
	"errors"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/protolambda/rumor/p2p/addrutil"
	"github.com/protolambda/zrnt/eth2/beacon"
	"net"
	"sync"
)

type EnrState struct {
	priv        *crypto.Secp256k1PrivateKey
	staticIP    net.IP
	fallbackUDP uint16
	fallbackIP  net.IP

	lock      sync.Mutex
	localNode *enode.LocalNode
}

// Create a new ENR state with a private key. The static / fallback choices are optional.
func NewEnrState(staticIP net.IP, fallbackIP net.IP, fallbackUDP uint16, privKey *crypto.Secp256k1PrivateKey) (*EnrState, error) {
	// TODO: persistent local node
	localNodeDB, err := enode.OpenDB("") // memory-DB
	if err != nil {
		return nil, err
	}
	if privKey == nil {
		return nil, errors.New("need private key for Discv5. Try 'host start' or 'enr make'")
	}
	localNode := enode.NewLocalNode(localNodeDB, (*ecdsa.PrivateKey)(privKey))
	if staticIP != nil {
		localNode.SetStaticIP(staticIP)
	}
	if fallbackIP != nil {
		localNode.SetFallbackIP(fallbackIP)
	}
	if fallbackUDP != 0 {
		localNode.SetFallbackUDP(int(fallbackUDP))
	}
	// TODO: load other eth2 entries
	return &EnrState{
		priv:        privKey,
		staticIP:    staticIP,
		fallbackUDP: fallbackUDP,
		fallbackIP:  fallbackIP,
		localNode:   localNode,
	}, nil
}

func (s *EnrState) LocalNode() *enode.LocalNode {
	return s.localNode
}

// StaticIP returns the current static IP, or nil if none is set.
func (s *EnrState) StaticIP() net.IP {
	return s.staticIP
}

func (s *EnrState) SetStaticIP(ip net.IP) {
	s.staticIP = ip
	s.localNode.SetStaticIP(ip)
}

// FallbackIP returns the current static IP, or nil if none is set.
func (s *EnrState) FallbackIP() net.IP {
	return s.fallbackIP
}

func (s *EnrState) SetFallbackIP(ip net.IP) {
	s.fallbackIP = ip
	s.localNode.SetFallbackIP(ip)
}

// FallbackUDP returns the current static IP, or nil if none is set.
func (s *EnrState) FallbackUDP() uint16 {
	return s.fallbackUDP
}

func (s *EnrState) SetFallbackUDP(port uint16) {
	s.fallbackUDP = port
	s.localNode.SetFallbackUDP(int(port))
}

func (s *EnrState) GetNode() *enode.Node {
	return s.localNode.Node()
}

func (s *EnrState) GetPriv() *crypto.Secp256k1PrivateKey {
	return s.priv
}

func (s *EnrState) SetTCP(port uint16) {
	s.localNode.Set(enr.TCP(port))
}

func (s *EnrState) SetUDP(port uint16) {
	s.localNode.Set(enr.UDP(port))
}

func (s *EnrState) SetIP(ip net.IP) {
	if ip == nil {
		s.localNode.Delete(enr.IP(net.IPv4zero))
		s.localNode.Delete(enr.IP(net.IPv6zero))
		return
	}
	s.localNode.Set(enr.IP(ip))
}

func (s *EnrState) SetEth2Data(dat *beacon.Eth2Data) {
	s.localNode.Set(addrutil.NewEth2DataEntry(dat))
}

func (s *EnrState) SetAttnets(dat *beacon.AttnetBits) {
	s.localNode.Set(addrutil.NewAttnetsENREntry(dat))
}
