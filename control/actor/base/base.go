package base

import (
	"context"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/sirupsen/logrus"
	"io"
)

// Step is a function that is proposed by the command, and can be stepped into by the controller.
// The context is used to limit the step execution
type Step func(ctx context.Context) error

// OnStop is a function  that is proposed by the command, and called by the controller to end the command.
// The context is used to limit the stop execution
type OnStop func(ctx context.Context) error

type Control interface {
	// RegisterStop register a callback for the controller to shutdown a background task of a command
	// The command can be running the background task in some other go routine after returning.
	RegisterStop(onStop OnStop)

	// Step creates a job that can be stepped into by the controller.
	// It blocks until the step is consumed. The step itself can schedule next steps
	Step(step Step) error
}

type Base struct {
	WithHost
	// Shared between actors
	GlobalContext context.Context
	// For actor
	ActorContext context.Context
	// Control the execution of a call
	Control Control
	// For command
	Log logrus.FieldLogger
	// Optional std output
	Out io.Writer
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
