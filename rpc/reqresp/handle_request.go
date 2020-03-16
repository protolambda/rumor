package reqresp

import (
	"context"
	"encoding/binary"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"io"
	"time"
)

// RequestPayloadHandler processes a request (decompressed if previously compressed), read from r.
// The handler can respond by writing to w. After returning the writer will automatically be closed.
type RequestPayloadHandler func(ctx context.Context, peerId peer.ID, requestLen uint64, r io.Reader, w io.Writer)

type OnError func(ctx context.Context, peerId peer.ID, err error)

type StreamCtxFn func() context.Context

// startReqRPC registers a request handler for the given protocol. Compression is optional and may be nil.
func (handle RequestPayloadHandler) MakeStreamHandler(newCtx StreamCtxFn, comp Compression, onInvalidInput OnError, maxRequestContentSize int) network.StreamHandler {
	return func(stream network.Stream) {
		defer stream.Close()
		peerId := stream.Conn().RemotePeer()
		ctx, cancel := context.WithCancel(newCtx())
		defer cancel()

		blr := NewBufLimitReader(stream, 1024, 0)
		blr.N = 10  // var ints should be small
		reqLen, err := binary.ReadUvarint(blr)
		if err != nil {
			onInvalidInput(ctx, peerId, err)
			return
		}
		blr.N = maxRequestContentSize
		if err := stream.SetReadDeadline(time.Now().Add(time.Second * 10)); err != nil {
			onInvalidInput(ctx, peerId, err)
			return
		}
		r := io.Reader(blr)
		w := io.WriteCloser(stream)
		if comp != nil {
			r = comp.Decompress(r)
			w = comp.Compress(w)
			defer w.Close()
		}
		handle(ctx, peerId, reqLen, r, w)
	}
}
