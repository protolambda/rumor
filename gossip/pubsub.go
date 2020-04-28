package gossip

import (
	"context"
	"encoding/base64"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/minio/sha256-simd"
)

type GossipSub interface {
	Join(topic string, opts ...pubsub.TopicOpt) (*pubsub.Topic, error)
	BlacklistPeer(id peer.ID)
}

type gossipImpl struct {
	*pubsub.PubSub
}

func NewGossipSub(ctx context.Context, h host.Host) (GossipSub, error) {
	psOptions := []pubsub.Option{
		pubsub.WithMessageSigning(false),
		pubsub.WithStrictSignatureVerification(false),
		pubsub.WithMessageIdFn(MsgIDFunction),
	}
	ps, err := pubsub.NewGossipSub(ctx, h, psOptions...)
	if err != nil {
		return nil, err
	}
	return &gossipImpl{PubSub: ps}, nil
}

func MsgIDFunction(pmsg *pubsub_pb.Message) string {
	h := sha256.New()
	// never errors, see crypto/sha256 Go doc
	_, _ = h.Write(pmsg.Data)
	id := h.Sum(nil)
	return base64.URLEncoding.EncodeToString(id)
}
