package base

import (
	"context"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/sirupsen/logrus"
	"net"
)

type Base struct {
	WithHost
	BaseContext context.Context
	Log         logrus.FieldLogger
}

type WithHost interface {
	Host() (h host.Host, err error)
	GetEnr() *enr.Record
	GetIP() net.IP
	GetTCP() uint16
	GetUDP() uint16
}
