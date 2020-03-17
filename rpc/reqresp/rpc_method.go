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
	"reflect"
)

type Request interface {
	fmt.Stringer
}

type Codec interface {
	MaxByteLen() uint64
	Encode(w io.Writer, input interface{}) error
	Decode(r io.Reader, bytesLen uint64, dest interface{}) error
	Alloc() interface{}
}

type SSZCodec struct {
	def ztypes.SSZ
	alloc func() interface{}
}

func NewSSZCodec(typ interface{}) *SSZCodec {
	sszDef := zssz.GetSSZ(typ)
	rTyp := reflect.TypeOf(typ).Elem()
	alloc := func() interface{} {
		return reflect.New(rTyp).Interface()
	}
	return &SSZCodec{
		def:   sszDef,
		alloc: alloc,
	}
}

func (c *SSZCodec) MaxByteLen() uint64 {
	return c.def.MaxLen()
}

func (c *SSZCodec) Encode(w io.Writer, input interface{}) error {
	_, err := zssz.Encode(w, input, c.def)
	return err
}

func (c *SSZCodec) Decode(r io.Reader, bytesLen uint64, dest interface{}) error {
	return zssz.Decode(r, bytesLen, dest, c.def)
}

func (c *SSZCodec) Alloc() interface{} {
	return c.alloc()
}

type RPCMethod struct {
	Protocol           protocol.ID
	MaxChunkCount      uint64
	RequestCodec       Codec
	ResponseChunkCodec Codec
}

type ResponseCode uint8

const (
	SuccessCode    ResponseCode = 0
	InvalidReqCode              = 1
	ServerErrCode               = 2
)

// 8 KB max error size
const MAX_ERR_SIZE = 1 << 15

type MethodRespSuccessHandler func(chunkIndex uint64, readChunk func(dest interface{}) error) error
type MethodRespErrorHandler func(chunkIndex uint64, msg string) error

func (m *RPCMethod) RunRequest(ctx context.Context, newStreamFn NewStreamFn,
	peerId peer.ID, comp Compression, req interface{}, onResponse MethodRespSuccessHandler,
	onInvalidReqResp MethodRespErrorHandler, onServerErrorResp MethodRespErrorHandler, onClose func()) error {

	defer onClose()

	handleChunks := ResponseChunkHandler(func(ctx context.Context, chunkIndex uint64, chunkSize uint64, result ResponseCode, r io.Reader, w io.Writer) error {
		switch result {
		case SuccessCode:
			return onResponse(chunkIndex, func(dest interface{}) error {
				return m.RequestCodec.Decode(r, chunkSize, dest)
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

	protocolId := m.Protocol
	maxChunkContentSize := m.ResponseChunkCodec.MaxByteLen()
	if comp != nil {
		protocolId += protocol.ID("_" + comp.Name())
		if s, err := comp.MaxEncodedLen(maxChunkContentSize); err != nil {
			return err
		} else {
			maxChunkContentSize = s
		}
	}

	respHandler := handleChunks.MakeResponseHandler(m.MaxChunkCount, maxChunkContentSize, comp)

	var buf bytes.Buffer
	if err := m.RequestCodec.Encode(&buf, req); err != nil {
		return err
	}
	// Runs the request in sync, which processes responses,
	// and then finally closes the channel through the earlier deferred close.
	return newStreamFn.Request(ctx, peerId, protocolId, &buf, comp, respHandler)
}

type ReadRequestFn func (dest interface{}) error
type WriteSuccessChunkFn func(data interface{}) error
type WriteMsgFn func(msg string) error

type RequestReader interface {
	// nil if not an invalid input
	InvalidInput() error
	ReadRequest(dest interface{}) error
	RawRequest() ([]byte, error)
}
type RequestResponder interface {
	WriteResponseChunk(data interface{}) error
	WriteRawResponseChunk(chunk []byte) error
	WriteInvalidMsgChunk(msg string) error
	WriteServerErrorChunk(msg string) error
}
type ChunkedRequestHandler interface {
	RequestReader
	RequestResponder
}

type chReqHandler struct {
	m *RPCMethod
	comp Compression
	respBuf bytes.Buffer
	reqLen uint64
	r io.Reader
	w io.Writer
	invalidInputErr error
}

func (h *chReqHandler) InvalidInput() error {
	return h.invalidInputErr
}

func (h *chReqHandler) ReadRequest(dest interface{}) error {
	if h.invalidInputErr != nil {
		return h.invalidInputErr
	}
	return h.m.RequestCodec.Decode(h.r, h.reqLen, dest)
}

func (h *chReqHandler) RawRequest() ([]byte, error) {
	if h.invalidInputErr != nil {
		return nil, h.invalidInputErr
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(h.r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (h *chReqHandler) WriteResponseChunk(data interface{}) error {
	h.respBuf.Reset()  // re-use buffer for each response chunk
	if err := h.m.ResponseChunkCodec.Encode(&h.respBuf, data); err != nil {
		return err
	}
	return EncodeChunk(SuccessCode, bytes.NewReader(h.respBuf.Bytes()), h.w, h.comp)
}

func (h *chReqHandler) WriteRawResponseChunk(chunk []byte) error {
	return EncodeChunk(SuccessCode, bytes.NewReader(chunk), h.w, h.comp)
}

func (h *chReqHandler) WriteInvalidMsgChunk(msg string) error {
	return EncodeChunk(InvalidReqCode, bytes.NewReader([]byte(msg)), h.w, h.comp)
}

func (h *chReqHandler) WriteServerErrorChunk(msg string) error {
	return EncodeChunk(ServerErrCode, bytes.NewReader([]byte(msg)), h.w, h.comp)
}

type OnRequestListener func(ctx context.Context, peerId peer.ID, handler ChunkedRequestHandler)

func (m *RPCMethod) MakeStreamHandler(newCtx StreamCtxFn, comp Compression, listener OnRequestListener) (network.StreamHandler, error) {
	return RequestPayloadHandler(func(ctx context.Context, peerId peer.ID, requestLen uint64, r io.Reader, w io.Writer, invalidInputErr error) {
		listener(ctx, peerId, &chReqHandler{
			m: m, comp: comp, reqLen: requestLen, r: r, w: w, invalidInputErr: invalidInputErr,
		})
	}).MakeStreamHandler(newCtx, comp, m.RequestCodec.MaxByteLen()), nil
}
