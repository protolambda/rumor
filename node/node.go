package node

import (
	"github.com/libp2p/go-libp2p-core/host"
)

type Node interface {
	Host() host.Host
}
