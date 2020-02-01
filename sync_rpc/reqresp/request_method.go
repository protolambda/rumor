package reqresp

import (
	"context"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/protolambda/zssz"
	ztypes "github.com/protolambda/zssz/types"
	"io"
)

type RPCMethod struct {
	Protocol protocol.ID
	MaxChunkCount uint64
	ReqSSZ ztypes.SSZ
	RespChunkSSZ ztypes.SSZ
}

type RespChunkEntry struct {
	ChunkIndex uint64
	ResponseCode uint8
	ReadChunk func(dest interface{}) error
}

func (reqType *RPCMethod) RunRequest(ctx context.Context, newStreamFn NewStreamFn,
	peerId peer.ID, comp Compression, req interface{}, responses chan<- RespChunkEntry) error {

	defer func() {
		close(responses)
	}()

	handleChunks := ResponseChunkHandler(func(ctx context.Context, chunkIndex uint64, chunkSize uint64, result uint8, r io.Reader, w io.Writer) error {
		responses <- RespChunkEntry{ChunkIndex: chunkIndex, ResponseCode: result, ReadChunk: func(dest interface{}) error {
			return zssz.Decode(r, chunkSize, dest, reqType.RespChunkSSZ)
		}}
		return nil
	})

	protocolId := reqType.Protocol
	maxChunkSize := reqType.RespChunkSSZ.MaxLen()
	if comp != nil {
		protocolId += protocol.ID("_" + comp.Name())
		if s, err := comp.MaxEncodedLen(maxChunkSize); err != nil {
			return err
		} else {
			maxChunkSize = s
		}
	}

	respHandler := handleChunks.MakeResponseHandler(reqType.MaxChunkCount, maxChunkSize, comp)

	// Runs the request in sync, which processes responses,
	// and then finally closes the channel through the earlier deferred close.
	return newStreamFn.Request(ctx, peerId, protocolId, req, reqType.ReqSSZ, comp, respHandler)
}
