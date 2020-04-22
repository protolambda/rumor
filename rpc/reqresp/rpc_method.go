package reqresp

import (
	"bytes"
	"context"
	"errors"
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
	def   ztypes.SSZ
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
	if c == nil {
		return 0
	}
	return c.def.MaxLen()
}

func (c *SSZCodec) Encode(w io.Writer, input interface{}) error {
	if c == nil {
		if input != nil {
			return errors.New("expected empty data, nil input. This codec is no-op")
		}
		return nil
	}
	_, err := zssz.Encode(w, input, c.def)
	return err
}

func (c *SSZCodec) Decode(r io.Reader, bytesLen uint64, dest interface{}) error {
	if c == nil {
		if bytesLen != 0 {
			return errors.New("expected 0 bytes, no definition")
		}
		return nil
	}
	return zssz.Decode(r, bytesLen, dest, c.def)
}

func (c *SSZCodec) Alloc() interface{} {
	if c == nil {
		return nil
	}
	return c.alloc()
}

type RPCMethod struct {
	Protocol                  protocol.ID
	RequestCodec              Codec
	ResponseChunkCodec        Codec
	DefaultResponseChunkCount uint64
}

type ResponseCode uint8

const (
	SuccessCode    ResponseCode = 0
	InvalidReqCode              = 1
	ServerErrCode               = 2
)

// 8 KB max error size
const MAX_ERR_SIZE = 1 << 15

type OnResponseListener func(chunk ChunkedResponseHandler) error

type RequestInput interface {
	Reader(c Codec) (io.Reader, error)
}

type RequestSSZInput struct {
	Obj interface{}
}

func (v RequestSSZInput) Reader(c Codec) (io.Reader, error) {
	var buf bytes.Buffer
	if err := c.Encode(&buf, v.Obj); err != nil {
		return nil, err
	}
	return &buf, nil
}

type RequestBytesInput []byte

func (v RequestBytesInput) Reader(_ Codec) (io.Reader, error) {
	return bytes.NewReader(v), nil
}

type ChunkedResponseHandler interface {
	ChunkSize() uint64
	ChunkIndex() uint64
	ResultCode() ResponseCode
	ReadRaw() ([]byte, error)
	ReadErrMsg() (string, error)
	ReadObj(dest interface{}) error
}

type chRespHandler struct {
	m          *RPCMethod
	r          io.Reader
	result     ResponseCode
	chunkSize  uint64
	chunkIndex uint64
}

func (c *chRespHandler) ChunkSize() uint64 {
	return c.chunkSize
}

func (c *chRespHandler) ChunkIndex() uint64 {
	return c.chunkIndex
}

func (c *chRespHandler) ResultCode() ResponseCode {
	return c.result
}

func (c *chRespHandler) ReadRaw() ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(io.LimitReader(c.r, int64(c.chunkSize)))
	return buf.Bytes(), err
}

func (c *chRespHandler) ReadErrMsg() (string, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(io.LimitReader(c.r, int64(c.chunkSize)))
	return string(buf.Bytes()), err
}

func (c *chRespHandler) ReadObj(dest interface{}) error {
	return c.m.ResponseChunkCodec.Decode(c.r, c.chunkSize, dest)
}

func (m *RPCMethod) RunRequest(ctx context.Context, newStreamFn NewStreamFn,
	peerId peer.ID, comp Compression, req RequestInput, maxRespChunks uint64, onResponse OnResponseListener) error {

	handleChunks := ResponseChunkHandler(func(ctx context.Context, chunkIndex uint64, chunkSize uint64, result ResponseCode, r io.Reader, w io.Writer) error {
		return onResponse(&chRespHandler{
			m:          m,
			r:          r,
			result:     result,
			chunkSize:  chunkSize,
			chunkIndex: chunkIndex,
		})
	})

	reqR, err := req.Reader(m.RequestCodec)
	if err != nil {
		return err
	}

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
	// TODO: make compression optional, depending on if the other peer supports it.
	// Pass multiple protocol ids, then check the protocol of the stream, and pick the suitable compression.

	respHandler := handleChunks.MakeResponseHandler(maxRespChunks, maxChunkContentSize, MAX_ERR_SIZE, comp)

	// Runs the request in sync, which processes responses,
	// and then finally closes the channel through the earlier deferred close.
	return newStreamFn.Request(ctx, peerId, protocolId, reqR, comp, respHandler)
}

type ReadRequestFn func(dest interface{}) error
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
	WriteInvalidRequestChunk(msg string) error
	WriteServerErrorChunk(msg string) error
}
type ChunkedRequestHandler interface {
	RequestReader
	RequestResponder
}

type chReqHandler struct {
	m               *RPCMethod
	comp            Compression
	respBuf         bytes.Buffer
	reqLen          uint64
	r               io.Reader
	w               io.Writer
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
	h.respBuf.Reset() // re-use buffer for each response chunk
	if err := h.m.ResponseChunkCodec.Encode(&h.respBuf, data); err != nil {
		return err
	}
	return EncodeChunk(SuccessCode, bytes.NewReader(h.respBuf.Bytes()), h.w, h.comp)
}

func (h *chReqHandler) WriteRawResponseChunk(chunk []byte) error {
	return EncodeChunk(SuccessCode, bytes.NewReader(chunk), h.w, h.comp)
}

func (h *chReqHandler) WriteInvalidRequestChunk(msg string) error {
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
