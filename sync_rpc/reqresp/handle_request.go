package reqresp

import (
	"context"
	"github.com/libp2p/go-libp2p-core/network"
	"io"
)

// RequestHandler processes a request (decompressed if previously compressed), read from r.
// The handler can respond by writing to w. After returning the writer will automatically be closed.
type RequestHandler func(ctx context.Context, r io.Reader, w io.Writer)

type StreamCtxFn func() context.Context

// startReqRPC registers a request handler for the given protocol. Compression is optional and may be nil.
func (handle RequestHandler) MakeStreamHandler(newCtx StreamCtxFn, comp Compression) network.StreamHandler {
	return func(stream network.Stream) {
		defer stream.Close()
		ctx, cancel := context.WithCancel(newCtx())
		defer cancel()
		r := io.Reader(stream)
		w := io.WriteCloser(stream)
		if comp != nil {
			r = comp.Decompress(r)
			w = comp.Compress(w)
			defer w.Close()
		}
		handle(ctx, r, w)
	}
}
