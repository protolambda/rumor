package reqresp

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
)

// ResponseChunkHandler is a function that processes a response chunk. The index, size and result-code are already parsed.
// The contents (decompressed if previously compressed) can be read from r. Optionally an answer can be written back to w.
// If the response chunk could not be processed, an error may be returned.
type ResponseChunkHandler func(ctx context.Context, chunkIndex uint64, chunkSize uint64, result ResponseCode, r io.Reader, w io.Writer) error

// ResponseHandler processes a response by internally processing chunks, any error is propagated up.
type ResponseHandler func(ctx context.Context, r io.Reader, w io.WriteCloser) error

// MakeResponseHandler builds a ResponseHandler, which won't take more than maxChunkCount chunks, or chunk contents larger than maxChunkContentSize.
// Compression is optional and may be nil. Chunks are processed by the given ResponseChunkHandler.
func (handleChunk ResponseChunkHandler) MakeResponseHandler(maxChunkCount uint64, maxChunkContentSize uint64, comp Compression) ResponseHandler {
	//		response  ::= <response_chunk>*
	//		response_chunk  ::= <result> | <encoding-dependent-header> | <encoded-payload>
	//		result    ::= “0” | “1” | “2” | [“128” ... ”255”]
	return func(ctx context.Context, r io.Reader, w io.WriteCloser) error {
		blr := NewBufLimitReader(r, 1024, 0)
		defer w.Close()
		for chunkIndex := uint64(0); chunkIndex < maxChunkCount; chunkIndex++ {
			blr.N = 1
			resByte, err := blr.ReadByte()
			if err == io.EOF { // no more chunks left.
				return nil
			}
			if err != nil {
				return fmt.Errorf("failed to read chunk %d result byte: %v", chunkIndex, err)
			}
			blr.N = 10
			chunkSize, err := binary.ReadUvarint(blr)
			if err != nil {
				// TODO send error back: invalid chunk size encoding
				return err
			}
			if chunkSize > maxChunkContentSize {
				// TODO sender error back: invalid chunk size, too large.
				return fmt.Errorf("chunk size %d of chunk %d exceeds chunk limit %d", chunkSize, chunkIndex, maxChunkContentSize)
			}
			blr.N = int(maxChunkContentSize)
			cr := io.Reader(blr)
			cw := w
			if comp != nil {
				cr = comp.Decompress(cr)
				cw = comp.Compress(cw)
			}
			if err := handleChunk(ctx, chunkIndex, chunkSize, ResponseCode(resByte), cr, cw); err != nil {
				_ = cw.Close()
				return err
			}
			if comp != nil {
				if err := cw.Close(); err != nil {
					return fmt.Errorf("failed to close response writer for chunk")
				}
			}
		}
		return nil
	}
}
