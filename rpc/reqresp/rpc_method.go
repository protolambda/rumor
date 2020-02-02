package reqresp

import (
	"bytes"
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/protolambda/zssz"
	ztypes "github.com/protolambda/zssz/types"
	"io"
)

type AllocRequest func() interface{}

type RPCMethod struct {
	Protocol      protocol.ID
	MaxChunkCount uint64
	ReqSSZ        ztypes.SSZ
	RespChunkSSZ  ztypes.SSZ
	AllocRequest  AllocRequest  // TODO update zssz to include this functionality
}

type RespChunkEntry struct {
	ChunkIndex   uint64
	ResponseCode uint8
	ReadChunk    func(dest interface{}) error
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

type WriteChunkFn func(responseCode uint8, data interface{}) error

type ChunkedRequestHandler func(ctx context.Context, peerId peer.ID, request interface{}, respChunk WriteChunkFn) error


func (reqType *RPCMethod) MakeStreamHandler(newCtx StreamCtxFn, comp Compression, handle ChunkedRequestHandler,
	onInvalidInput OnError, onServerError OnError) (network.StreamHandler, error) {

	reqSizeLimit := reqType.ReqSSZ.MaxLen()
	if comp != nil {
		s, err := comp.MaxEncodedLen(reqSizeLimit)
		if err != nil {
			return nil, err
		}
		reqSizeLimit = s
	}
	return RequestPayloadHandler(func(ctx context.Context, peerId peer.ID, requestLen uint64, r io.Reader, w io.Writer) {
		if requestLen > reqSizeLimit {
			onInvalidInput(ctx, peerId, fmt.Errorf("request length %d exceeds request size limit %d", requestLen, reqSizeLimit))
		}
		reqObj := reqType.AllocRequest()

		if comp != nil {
			r = comp.Decompress(r)
		}
		if err := zssz.Decode(r, requestLen, reqObj, reqType.ReqSSZ); err != nil {
			onInvalidInput(ctx, peerId, err)
			return
		}
		var buf bytes.Buffer

		if err := handle(ctx, peerId, reqObj, func(responseCode uint8, data interface{}) error {
			if _, err := zssz.Encode(&buf, data, reqType.RespChunkSSZ); err != nil {
				return err
			}
			return EncodeChunk(responseCode, &buf, w, comp)
		}); err != nil {
			onServerError(ctx, peerId, err)
			return
		}
	}).MakeStreamHandler(newCtx, comp, onInvalidInput), nil
}
