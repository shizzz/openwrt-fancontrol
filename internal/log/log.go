// ===== file: internal/log/log.go =====

package log

import (
	"fmt"
	"os"
	"time"
)

// Logger is a minimal structured logger that writes to stdout.
// On OpenWrt, stdout of a procd service is captured by logd and
// available via `logread`, so no additional syslog integration is needed.
type Logger struct {
	debug bool
}

// New creates a Logger. When debug is true, Debug-level messages are emitted.
func New(debug bool) *Logger {
	return &Logger{debug: debug}
}

func (l *Logger) log(level, format string, args ...any) {
	ts := time.Now().Format("2006-01-02T15:04:05Z07:00")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stdout, "%s [%s] %s\n", ts, level, msg)
}

// Infof logs an informational message.
func (l *Logger) Infof(format string, args ...any) {
	l.log("INFO", format, args...)
}

// Errorf logs an error message.
func (l *Logger) Errorf(format string, args ...any) {
	l.log("ERROR", format, args...)
}

// Debugf logs a debug message; suppressed unless debug mode is enabled.
func (l *Logger) Debugf(format string, args ...any) {
	if l.debug {
		l.log("DEBUG", format, args...)
	}
}

// Warnf logs a warning message.
func (l *Logger) Warnf(format string, args ...any) {
	l.log("WARN", format, args...)
}

// Package-level helpers used before a Logger is constructed (e.g. in main).

func Errorf(format string, args ...any) {
	ts := time.Now().Format("2006-01-02T15:04:05Z07:00")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s [ERROR] %s\n", ts, msg)
}