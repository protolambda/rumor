package discv5

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"eth2-lurk/node"
	"fmt"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/sirupsen/logrus"
	"net"
)

type Discv5 interface {
	AddDiscV5BootNodes(bootNodes []*discv5.Node) error
}

type Discv5Impl struct {
	net *discv5.Network
	log logrus.FieldLogger
}

func NewDiscV5(ctx context.Context, n node.Node, addr string, privKey *ecdsa.PrivateKey) (Discv5, error) {
	dv5Log := n.Logger("discv5")

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	dv5Log = dv5Log.WithField("addr", udpAddr)

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		dv5Log.Debugf("UDP listener start err: %v", err)
		return nil, err
	}
	dv5Log.Debug("UDP listener up")

	dv5Net, err := discv5.ListenUDP(privKey, conn, "", nil)
	if err != nil {
		dv5Log.Debugf("Discv5 listener start err: %v", err)
		return nil, err
	}
	dv5Log.Debug("Discv5 listener up")

	go func() {
		<-ctx.Done()
		dv5Log.Info("closing discv5", addr)
		dv5Net.Close()
		dv5Log.Info("closed discv5", addr)
	}()

	return &Discv5Impl{
		log: dv5Log,
		net:    dv5Net,
	}, nil
}

func (dv5 *Discv5Impl) AddDiscV5BootNodes(bootNodes []*discv5.Node) error {
	for _, v := range bootNodes {
		dv5.log.Info("adding discv5 bootnode: ", v.String())
	}
	return dv5.net.SetFallbackNodes(bootNodes)
}

func ParseEnr(v string) (*enr.Record, error) {
	data, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return nil, err
	}
	var record enr.Record
	if err := rlp.Decode(bytes.NewReader(data), &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func EnrToEnode(record *enr.Record, verifySig bool) (*enode.Node, error) {
	idSchemeName := record.IdentityScheme()

	if verifySig {
		if err := record.VerifySignature(enode.ValidSchemes[idSchemeName]); err != nil {
			return nil, err
		}
	}

	return enode.New(enode.ValidSchemes[idSchemeName], record)
}

func EnodeToDiscv5Node(en *enode.Node) (*discv5.Node, error) {
	id := discv5.PubkeyID(en.Pubkey())
	ip := en.IP()
	udpPort, tcpPort := uint16(en.UDP()), uint16(en.TCP())
	if ip == nil || udpPort == 0 || tcpPort == 0 {
		return nil, fmt.Errorf("enode record %v has missing ip/udp/tcp", en.String())
	}
	return &discv5.Node{IP: ip, UDP: udpPort, TCP: tcpPort, ID: id}, nil
}

func ParseDiscv5ENRs(enrAddrs []string) ([]*discv5.Node, error) {
	dv5Nodes := make([]*discv5.Node, 0, len(enrAddrs))
	for _, addr := range enrAddrs {
		enrRec, err := ParseEnr(addr)
		if err != nil {
			return nil, err
		}
		enodeAddr, err := EnrToEnode(enrRec, true)
		if err != nil {
			return nil, err
		}
		dv5Node, err := EnodeToDiscv5Node(enodeAddr)
		if err != nil {
			return nil, err
		}
		dv5Nodes = append(dv5Nodes, dv5Node)
	}
	return dv5Nodes, nil
}
