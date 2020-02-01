package reqresp

import (
	"bytes"
	"encoding/binary"
	"io"
)

type payloadBuffer bytes.Buffer

func (p *payloadBuffer) Close() error {
	return nil
}

func (p *payloadBuffer) Write(b []byte) (n int, err error) {
	return (*bytes.Buffer)(p).Write(b)
}

func (p *payloadBuffer) OutputSizeVarint(w io.Writer) error {
	size := (*bytes.Buffer)(p).Len()
	sizeBytes := [binary.MaxVarintLen64]byte{}
	// TODO unsigned or signed var int?
	sizeByteLen := binary.PutVarint(sizeBytes[:], int64(size))
	_, err := w.Write(sizeBytes[:sizeByteLen])
	return err
}

func (p *payloadBuffer) WriteTo(w io.Writer) (n int64, err error) {
	return (*bytes.Buffer)(p).WriteTo(w)
}

// EncodePayload reads a payload, buffers (and optionally compresses) the payload,
// then computes the header-data (varint of byte size). And then writes header and payload.
func EncodePayload(r io.Reader, w io.Writer, comp Compression) error {
	var out io.Writer
	var buf payloadBuffer
	out = &buf
	if comp != nil {
		compressedWriter := comp.Compress(&buf)
		defer compressedWriter.Close()
		out = compressedWriter
	}
	if _, err := io.Copy(out, r); err != nil {
		return err
	}
	if err := buf.OutputSizeVarint(w); err != nil {
		return err
	}
	if _, err := buf.WriteTo(w); err != nil {
		return err
	}
	return nil
}

// EncodeResult writes the result code to the output writer.
func EncodeResult(result uint8, w io.Writer) error {
	_, err := w.Write([]byte{result})
	return err
}

// EncodeChunk reads (decompressed) response message from the msg io.Reader,
// and writes it as a chunk with given result code to the output writer. The compression is optional and may be nil.
func EncodeChunk(result uint8, r io.Reader, w io.Writer, comp Compression) error {
	if err := EncodeResult(result, w); err != nil {
		return err
	}
	return EncodePayload(r, w, comp)
}
