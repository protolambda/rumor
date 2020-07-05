package base

import (
	"context"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/sirupsen/logrus"
)

// SpawnFn spawns background tasks.
// Ctx is used to terminate spawned resources.
// Done is called after the spawned resources are fully freed.
type SpawnFn func() (ctx context.Context, done context.CancelFunc)

type Base struct {
	WithHost
	// Shared between actors
	GlobalContext context.Context
	// For actor
	ActorContext context.Context
	// For non-blocking tasks to end later. E.g. serving data in the background.
	SpawnContext SpawnFn
	// For command
	Log logrus.FieldLogger
}

type WithHost interface {
	Host() (h host.Host, err error)
}

type WithHostPriv interface {
	GetHostPriv() *crypto.Secp256k1PrivateKey
}

type WithEnrNode interface {
	GetNode() (n *enode.Node, ok bool)
}

type PrivSettings interface {
	// GetPriv, may be nil
	GetPriv() *crypto.Secp256k1PrivateKey
	SetPriv(p *crypto.Secp256k1PrivateKey) error
}
