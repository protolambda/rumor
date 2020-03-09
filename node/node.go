package node

import (
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/sirupsen/logrus"
)

type Logger interface {
	logrus.FieldLogger
}

type Node interface {
	Host() host.Host
}

