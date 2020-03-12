package gossip

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/minio/sha256-simd"
	"github.com/protolambda/rumor/node"
	"github.com/sirupsen/logrus"
	"io"
)

type GossipSub interface {
	JoinTopic(topic string) (*pubsub.Topic, error)
	BlacklistPeer(id peer.ID)
	LogTopic(ctx context.Context, loggerName string, top *pubsub.Topic, out chan<- PubSubMessage, outErr chan<- error) (err error)
}

type GossipSubImpl struct {
	ps *pubsub.PubSub
	log logrus.FieldLogger
}

func NewGossipSub(ctx context.Context, log logrus.FieldLogger, n node.Node) (GossipSub, error) {
	psOptions := []pubsub.Option{
		pubsub.WithMessageSigning(false),
		pubsub.WithStrictSignatureVerification(false),
		pubsub.WithMessageIdFn(msgIDFunction),
	}
	ps, err := pubsub.NewGossipSub(ctx, n.Host(), psOptions...)
	if err != nil {
		log.Errorf("failed to init GossipSub: %v", err)
		return nil, err
	}
	log.Info("started pubsub")
	return &GossipSubImpl{
		ps:  ps,
		log: log,
	}, nil
}

func msgIDFunction(pmsg *pubsub_pb.Message) string {
	h := sha256.New()
	// never errors, see crypto/sha256 Go doc
	_, _ = h.Write(pmsg.Data)
	id := h.Sum(nil)
	return base64.URLEncoding.EncodeToString(id)
}

type PubSubMessage struct {
	from peer.ID
	data []byte
}

func (gs *GossipSubImpl) JoinTopic(topic string) (*pubsub.Topic, error) {
	return gs.ps.Join(topic)
}

func (gs *GossipSubImpl) BlacklistPeer(id peer.ID) {
	gs.ps.BlacklistPeer(id)
}

func (gs *GossipSubImpl) LogTopic(ctx context.Context, loggerName string, top *pubsub.Topic, out chan<- PubSubMessage, outErr chan<- error) (err error) {
	log := gs.log.WithField("logger", loggerName)
	log.Info("starting logging on pubsub topic")
	sub, err := top.Subscribe()
	if err != nil {
		log.Errorf("failed to subscribe: %v", err)
		return err
	}
	running := true
	go func() {
		log.Info("start subscription")
		for {
			msg, err := sub.Next(ctx)
			if !running {
				break
			}
			if err != nil {
				outErr <- err
				continue
			}
			log.Debugf("Received message! logger: %s from %s: %d bytes, seq nr: %x", loggerName, msg.GetFrom().Pretty(), msg.Size(), msg.Seqno)
			out <- PubSubMessage{
				from: msg.GetFrom(),
				data: msg.Data,
			}
		}
		log.Info("stopped subscription")
	}()

	go func() {
		<-ctx.Done()
		log.Info("stopping listening")
		running = false
	}()

	return nil
}

func NewMessageLogger(ctx context.Context, w io.Writer, outErr chan<- error) chan<- PubSubMessage {
	data := make([]byte, 1<<14)
	outErr = make(chan error)
	onMsg := func(msg PubSubMessage) {
		n := hex.EncodedLen(len(msg.data))
		if n > len(data) {
			data = make([]byte, n*2, n*2)
		}
		hex.Encode(data, msg.data)
		if _, err := w.Write(data[:n]); err != nil {
			outErr <- err
		}
		if _, err := w.Write([]byte{'\n'}); err != nil {
			outErr <- err
		}
	}

	listenerCh := make(chan PubSubMessage)

	go func() {
		for {
			select {
			case msg := <-listenerCh:
				onMsg(msg)
			case <-ctx.Done():
				return
			}
		}
	}()
	return listenerCh
}

func NewErrLoggerChannel(ctx context.Context, log logrus.FieldLogger, name string) chan<- error {
	log = log.WithField("error_zone_name", name)
	listenerCh := make(chan error)
	go func() {
		for {
			select {
			case msg := <-listenerCh:
				log.Error(msg)
			case <-ctx.Done():
				return
			}
		}
	}()
	return listenerCh
}

