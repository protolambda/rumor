package control

import (
	"github.com/sirupsen/logrus"
	"io"
)

type WriteableFn func(msg string)

func (fn WriteableFn) Write(p []byte) (n int, err error) {
	fn(string(p))
	return len(p), nil
}

type LogSplitFn func(entry *logrus.Entry) error

func (fn LogSplitFn) Format(entry *logrus.Entry) ([]byte, error) {
	// we don't care about formatting the message contents, we just forward the entry itself elsewhere
	return nil, fn(entry)
}

type VoidWriter struct{}

func (v VoidWriter) Write(p []byte) (n int, err error) {
	return len(p), err
}

type NextSingleLineFn func() (string, error)

// Reads one or more lines at a time. The multi-line string may not end with a linebreak character,
// but should never end in the middle of an actual line.
type NextMultiLineFn func() (string, error)

// LinesReader is used to convert a stream of lines back into a reader
type LinesReader struct {
	Fn   NextMultiLineFn
	last []byte
}

func (lr *LinesReader) Read(p []byte) (n int, err error) {
	//fmt.Printf("<linesreader> started with p len %d\n", len(p))
	//defer func() {
	//	fmt.Printf("<linesreader> (n = %d, err = %v) \"%s\"\n", n, err, string(p))
	//}()
	for {
		// When we've a line-end, or have to stop otherwise, just end this read, next read will have new things.
		if len(p) == 0 || n >= len(p) || (n >= 1 && p[n-1] == '\n') {
			return
		}
		// If there are any buffered bytes, write those first
		if len(lr.last) > 0 {
			sk := copy(p[n:], lr.last)
			lr.last = lr.last[sk:]
			n += sk
			//fmt.Printf("<linesreader> added %d bytes, total %d, data = '%s'\n", sk, n, string(p))
		} else if n < len(p) { // Now if there is still place left, start reading the next line.
			out, err := lr.Fn()
			if err != nil {
				//fmt.Printf("<linesreader> err %v\n", err)
				return n, err
			}
			out += "\n"
			sk := copy(p[n:], out)
			n += sk
			//fmt.Printf("<linesreader> added %d bytes, total %d, data = '%s'\n", sk, n, string(p))
			// buffer any remainder
			lr.last = []byte(out)[sk:]
		} else {
			// if there are no bytes left to read into, end this read.
			//fmt.Printf("<linesreader> full\n")
			return n, nil
		}
	}
}

type ReadWithCallback struct {
	io.Reader
	Callback func() error
}

func (rc *ReadWithCallback) Read(p []byte) (n int, err error) {
	n, err = rc.Reader.Read(p)
	if err != nil {
		return
	}
	err = rc.Callback()
	return
}
