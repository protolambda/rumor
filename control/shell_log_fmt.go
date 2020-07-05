package control

import (
	"bytes"
	"fmt"
	. "github.com/sirupsen/logrus"
	"sort"
	"strings"
)

const LogKeyActor = "actor"
const LogKeyCallID = "call_id"

const (
	red    = 31
	yellow = 33
	blue   = 36
	gray   = 37
	cyan   = 96
)

const callColor int = yellow
const actorColor int = cyan

const timeFmt = "15:04:05"

// ShellLogFmt formats logs into text
type ShellLogFmt struct {
	FieldMap FieldMap
}

// Format renders a single log entry
func (f *ShellLogFmt) Format(entry *Entry) ([]byte, error) {
	data := make(Fields)
	for k, v := range entry.Data {
		data[k] = v
	}
	keys := make([]string, 0, len(data))
	for k := range data {
		if k == LogKeyActor || k == LogKeyCallID {
			continue
		}
		keys = append(keys, k)
	}

	var funcVal, fileVal string

	fixedKeys := make([]string, 0, 4+len(data))
	fixedKeys = append(fixedKeys, FieldKeyTime)
	fixedKeys = append(fixedKeys, FieldKeyLevel)
	if entry.Message != "" {
		fixedKeys = append(fixedKeys, FieldKeyMsg)
	}
	fixedKeys = append(fixedKeys, FieldKeyLogrusError)
	if entry.HasCaller() {
		funcVal = entry.Caller.Function
		fileVal = fmt.Sprintf("%s:%d", entry.Caller.File, entry.Caller.Line)

		if funcVal != "" {
			fixedKeys = append(fixedKeys, FieldKeyFunc)
		}
		if fileVal != "" {
			fixedKeys = append(fixedKeys, FieldKeyFile)
		}
	}

	sort.Strings(keys)
	fixedKeys = append(fixedKeys, keys...)

	var b *bytes.Buffer
	if entry.Buffer != nil {
		b = entry.Buffer
	} else {
		b = &bytes.Buffer{}
	}

	var levelColor int
	switch entry.Level {
	case DebugLevel, TraceLevel:
		levelColor = gray
	case WarnLevel:
		levelColor = yellow
	case ErrorLevel, FatalLevel, PanicLevel:
		levelColor = red
	default:
		levelColor = blue
	}

	levelText := strings.ToUpper(entry.Level.String())
	levelText = levelText[0:4]

	// Remove a single newline if it already exists in the message to keep
	// the behavior of logrus text_formatter the same as the stdlib log package
	entry.Message = strings.TrimSuffix(entry.Message, "\n")

	caller := ""
	if entry.HasCaller() {
		funcVal := fmt.Sprintf("%s()", entry.Caller.Function)
		fileVal := fmt.Sprintf("%s:%d", entry.Caller.File, entry.Caller.Line)

		if fileVal == "" {
			caller = funcVal
		} else if funcVal == "" {
			caller = fileVal
		} else {
			caller = fileVal + " " + funcVal
		}
	}

	if callID, ok := entry.Data[LogKeyCallID]; ok {
		fmt.Fprintf(b, "\033[%dm[%s]\033[0m", callColor, callID) // yellow
	}
	if actorID, ok := entry.Data[LogKeyActor]; ok {
		fmt.Fprintf(b, "\033[%dm[%s]\033[0m", actorColor, actorID) // cyan
	}
	fmt.Fprintf(b, "\x1b[%dm[%s]\x1b[0m[%s]%s %-44s ", levelColor, levelText, entry.Time.Format(timeFmt), caller, entry.Message)

	for _, k := range keys {
		v := data[k]
		fmt.Fprintf(b, " \x1b[%dm%s\x1b[0m=", levelColor, k)
		f.appendValue(b, v)
	}

	b.WriteByte('\n')
	return b.Bytes(), nil
}

func (f *ShellLogFmt) appendKeyValue(b *bytes.Buffer, key string, value interface{}) {
	if b.Len() > 0 {
		b.WriteByte(' ')
	}
	b.WriteString(key)
	b.WriteByte('=')
	f.appendValue(b, value)
}

func (f *ShellLogFmt) appendValue(b *bytes.Buffer, value interface{}) {
	stringVal, ok := value.(string)
	if !ok {
		stringVal = fmt.Sprint(value)
	}
	newline := strings.Contains(stringVal, "\n")
	if newline {
		// start multi-line strings on new lines
		b.WriteRune('\n')
	}
	b.WriteString(stringVal)
	if newline {
		// and end them on new lines
		b.WriteRune('\n')
	}
}
