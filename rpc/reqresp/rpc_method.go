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

type Request interface {
	fmt.Stringer
}

type AllocRequest func() Request

type RPCMethod struct {
	Protocol      protocol.ID
	MaxChunkCount uint64
	ReqSSZ        ztypes.SSZ
	RespChunkSSZ  ztypes.SSZ
	AllocRequest  AllocRequest  // TODO update zssz to include this functionality
}

type ResponseCode uint8

const (
	SuccessCode ResponseCode = 0
	InvalidReqCode = 1
	ServerErrCode = 2
)

// 8 KB max error size
const MAX_ERR_SIZE = 1 << 15

type MethodRespSuccessHandler func(chunkIndex uint64, readChunk func(dest interface{}) error) error
type MethodRespErrorHandler func(chunkIndex uint64, msg string) error

func (reqType *RPCMethod) RunRequest(ctx context.Context, newStreamFn NewStreamFn,
	peerId peer.ID, comp Compression, req Request, onResponse MethodRespSuccessHandler,
	onInvalidReqResp MethodRespErrorHandler, onServerErrorResp MethodRespErrorHandler, onClose func()) error {

	defer onClose()

	handleChunks := ResponseChunkHandler(func(ctx context.Context, chunkIndex uint64, chunkSize uint64, result ResponseCode, r io.Reader, w io.Writer) error {
		switch result {
		case SuccessCode:
			return onResponse(chunkIndex, func(dest interface{}) error {
				return zssz.Decode(r, chunkSize, dest, reqType.RespChunkSSZ)
			})
		case InvalidReqCode:
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(io.LimitReader(r, MAX_ERR_SIZE))
			return onInvalidReqResp(chunkIndex, string(buf.Bytes()))
		case ServerErrCode:
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(io.LimitReader(r, MAX_ERR_SIZE))
			return onServerErrorResp(chunkIndex, string(buf.Bytes()))
		default:
			return fmt.Errorf("unrecognized result code for chunk %d (size %d): %d from peer %s", chunkIndex, chunkSize, result, peerId.Pretty())
		}
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

type WriteSuccessChunkFn func(data interface{}) error
type WriteMsgFn func(msg string) error

type ChunkedRequestHandler func(ctx context.Context, peerId peer.ID, request Request, respChunk WriteSuccessChunkFn, respChunkInvalidInput WriteMsgFn, respChunkServerError WriteMsgFn) error


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
			return
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

		if err := handle(ctx, peerId, reqObj, func(data interface{}) error {
			if _, err := zssz.Encode(&buf, data, reqType.RespChunkSSZ); err != nil {
				return err
			}
			return EncodeChunk(SuccessCode, bytes.NewReader(buf.Bytes()), w, comp)
		}, func(msg string) error {
			return EncodeChunk(InvalidReqCode, bytes.NewReader([]byte(msg)), w, comp)
		}, func(msg string) error {
			return EncodeChunk(ServerErrCode, bytes.NewReader([]byte(msg)), w, comp)
		}); err != nil {
			onServerError(ctx, peerId, err)
			return
		}
	}).MakeStreamHandler(newCtx, comp, onInvalidInput), nil
}
