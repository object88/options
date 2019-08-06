package testing

import (
	"testing"

	"github.com/object88/options/log"
)

type Log struct {
	t *testing.T
}

func NewLog(t *testing.T) *Log {
	return &Log{
		t: t,
	}
}

// Debugf will write if the log level is at least Debug.
func (l *Log) Debugf(msg string, v ...interface{}) {
	l.t.Logf(msg, v...)
}

// Errorf will write if the log level is at least Error.
func (l *Log) Errorf(msg string, v ...interface{}) {
	l.t.Logf(msg, v...)
}

// Infof will write if the log level is at least Info.
func (l *Log) Infof(msg string, v ...interface{}) {
	l.t.Logf(msg, v...)
}

// Printf will always log the given message, regardless of log level set.
func (l *Log) Printf(msg string, v ...interface{}) {
	l.t.Logf(msg, v...)
}

// SetLevel will adjust the logger's level.
func (l *Log) SetLevel(lvl log.Level) {
	// Do nothing
}

// Verbosef will write if the log level is at least Verbose.
func (l *Log) Verbosef(msg string, v ...interface{}) {
	l.t.Logf(msg, v...)
}

// Warnf will write if the log level is at least Warn.
func (l *Log) Warnf(msg string, v ...interface{}) {
	l.t.Logf(msg, v...)
}
