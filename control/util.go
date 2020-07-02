package control

import "github.com/sirupsen/logrus"

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

type NextLineFn func() (string, error)
