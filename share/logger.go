package chshare

import (
	"fmt"
	"log"
	"os"
)

//Logger is pkg/log Logger with prefixing and 2 log levels
type Logger struct {
	prefix      string
	logger      *log.Logger
	Info, Debug bool
}

func NewLogger(prefix string) *Logger {
	return NewLoggerFlag(prefix, log.Ldate|log.Ltime)
}

func NewLoggerFlag(prefix string, flag int) *Logger {
	l := &Logger{
		prefix: prefix,
		logger: log.New(os.Stderr, "", flag),
		Info:   false,
		Debug:  false,
	}
	return l
}

func (l *Logger) Infof(f string, args ...interface{}) {
	if l.Info {
		l.logger.Printf(l.prefix+": "+f, args...)
	}
}

func (l *Logger) Debugf(f string, args ...interface{}) {
	if l.Debug {
		l.logger.Printf(l.prefix+": "+f, args...)
	}
}

func (l *Logger) Errorf(f string, args ...interface{}) error {
	return fmt.Errorf(l.prefix+": "+f, args...)
}

func (l *Logger) Fork(prefix string, args ...interface{}) *Logger {
	//slip the parent prefix at the front
	args = append([]interface{}{l.prefix}, args...)
	ll := NewLogger(fmt.Sprintf("%s: "+prefix, args...))
	ll.Info = l.Info
	ll.Debug = l.Debug
	return ll
}

func (l *Logger) Prefix() string {
	return l.prefix
}
