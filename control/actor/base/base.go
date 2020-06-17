package base

import (
	"context"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/sirupsen/logrus"
)

type Base struct {
	WithHost
	BaseContext context.Context
	Log         logrus.FieldLogger
}

type WithHost interface {
	Host() (h host.Host, err error)
	GetEnr
}
