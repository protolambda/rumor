package dv5

import (
	"encoding/json"
	"fmt"
	geth_log "github.com/ethereum/go-ethereum/log"
	"github.com/sirupsen/logrus"
	"reflect"
)

type GethLogger struct {
	logrus.FieldLogger
}

type LazyLogEntry struct {
	dat geth_log.Lazy
}

// From the Geth source, since it's not accessible and we somehow need to get the log values.
func evaluateLazy(lz geth_log.Lazy) (interface{}, error) {
	t := reflect.TypeOf(lz.Fn)

	if t.Kind() != reflect.Func {
		return nil, fmt.Errorf("INVALID_LAZY, not func: %+v", lz.Fn)
	}

	if t.NumIn() > 0 {
		return nil, fmt.Errorf("INVALID_LAZY, func takes args: %+v", lz.Fn)
	}

	if t.NumOut() == 0 {
		return nil, fmt.Errorf("INVALID_LAZY, no func return val: %+v", lz.Fn)
	}

	value := reflect.ValueOf(lz.Fn)
	results := value.Call([]reflect.Value{})
	if len(results) == 1 {
		return results[0].Interface(), nil
	}
	values := make([]interface{}, len(results))
	for i, v := range results {
		values[i] = v.Interface()
	}
	return values, nil
}

func (lle *LazyLogEntry) MarshalJSON() ([]byte, error) {
	dat, err := evaluateLazy(lle.dat)
	if err != nil {
		return nil, err
	}
	return json.Marshal(dat)
}

func (lle *LazyLogEntry) MarshalText() ([]byte, error) {
	dat, err := evaluateLazy(lle.dat)
	if err != nil {
		return nil, err
	}
	return json.Marshal(dat)
}

// New returns a new Logger that has this logger's context plus the given context
func (gl *GethLogger) Log(r *geth_log.Record) error {
	rCtx := r.Ctx
	l := gl.FieldLogger
	for i := 0; i < len(rCtx); i += 2 {
		val := rCtx[i+1]
		if inner, ok := val.(geth_log.Lazy); ok {
			val = &LazyLogEntry{inner}
		}
		l = l.WithField(rCtx[i].(string), val)
	}
	switch r.Lvl {
	case geth_log.LvlCrit:
		l.Panicln(r.Msg)
	case geth_log.LvlError:
		l.Errorln(r.Msg)
	case geth_log.LvlWarn:
		l.Warningln(r.Msg)
	case geth_log.LvlInfo:
		l.Infoln(r.Msg)
	case geth_log.LvlDebug:
		l.Debugln(r.Msg)
	case geth_log.LvlTrace:
		l.Debugln(r.Msg)
	}
	return nil
}
