package actor

import (
	"github.com/sirupsen/logrus"
	"sync"
)

// Wrapping the regular logger, to make modifications after passing it to other places.
type Logger struct {
	logrus.FieldLogger
	m sync.Mutex
}

func NewLogger(inner logrus.FieldLogger) *Logger {
	return &Logger{
		FieldLogger: inner,
	}
}

func (l *Logger) Modify(modFn func(old logrus.FieldLogger) (new logrus.FieldLogger))  {
	l.m.Lock()
	defer l.m.Unlock()
	l.FieldLogger = modFn(l.FieldLogger)
}
