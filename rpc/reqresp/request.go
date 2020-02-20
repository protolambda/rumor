package reqresp

import (
	"bytes"
	"context"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/protolambda/zssz"
	ztypes "github.com/protolambda/zssz/types"
	"time"
)

type NewStreamFn func(ctx context.Context, peerId peer.ID, protocolId protocol.ID) (network.Stream, error)

func (newStreamFn NewStreamFn) WithTimeout(timeout time.Duration) NewStreamFn {
	return func(ctx context.Context, peerId peer.ID, protocolId protocol.ID) (network.Stream, error) {
		deadline := time.Now().Add(timeout)
		stream, err := newStreamFn(ctx, peerId, protocolId)
		if err != nil {
			return nil, err
		}
		if err := stream.SetReadDeadline(deadline); err != nil {
			return nil, err
		}
		if err := stream.SetWriteDeadline(deadline); err != nil {
			return nil, err
		}
		return stream, nil
	}
}

func (newStreamFn NewStreamFn) Request(ctx context.Context, peerId peer.ID, protocolId protocol.ID, data Request,
	dataType ztypes.SSZ, comp Compression, handle ResponseHandler) error {

	var buf bytes.Buffer
	if _, err := zssz.Encode(&buf, data, dataType); err != nil {
		return err
	}
	stream, err := newStreamFn(ctx, peerId, protocolId)
	if err != nil {
		return nil
	}
	var buf2 bytes.Buffer
	if err := EncodePayload(bytes.NewReader(buf.Bytes()), &buf2, comp); err != nil {
		return err
	}
	if _, err := stream.Write(buf2.Bytes()); err != nil {
		return err
	}
	return handle(ctx, stream, stream)
}
