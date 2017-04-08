// Package logger provides support for logging to stdout and stderr.
package logger

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/pengsrc/go-utils/convert"
	"github.com/pengsrc/go-utils/reopen"
)

// LogFormatter is used to format log entry.
type LogFormatter struct{}

// Format formats a given log entry, returns byte slice and error.
func (c *LogFormatter) Format(entry *log.Entry) ([]byte, error) {
	level := strings.ToUpper(entry.Level.String())
	if level == "WARNING" {
		level = "WARN"
	}
	if len(level) < 5 {
		level = strings.Repeat(" ", 5-len(level)) + level
	}

	return []byte(
		fmt.Sprintf(
			"[%s #%d] %s -- : %s\n",
			convert.TimeToString(time.Now(), convert.ISO8601Milli),
			os.Getpid(),
			level,
			entry.Message,
		),
	), nil
}

// NewLogFormatter creates a new log formatter.
func NewLogFormatter() *LogFormatter {
	return &LogFormatter{}
}

// ErrorHook presents error hook.
type ErrorHook struct {
	levels []log.Level

	out       io.Writer
	formatter log.Formatter
}

// Levels returns error log levels.
func (eh *ErrorHook) Levels() []log.Level {
	return eh.levels
}

// Fire triggers before logging.
func (eh *ErrorHook) Fire(entry *log.Entry) error {
	formatted, err := eh.formatter.Format(entry)
	if err != nil {
		return err
	}
	_, err = eh.out.Write(formatted)
	if err != nil {
		return err
	}
	return nil
}

// NewErrorHook creates new error hook.
func NewErrorHook(out io.Writer) *ErrorHook {
	return &ErrorHook{
		levels: []log.Level{
			log.WarnLevel,
			log.ErrorLevel,
			log.FatalLevel,
			log.PanicLevel,
		},
		out:       out,
		formatter: NewLogFormatter(),
	}
}

// Logger presents a logger.
type Logger struct {
	origLogger *log.Logger

	out         io.Writer
	bufferedOut Flusher
}

// Flusher defines a interface with Flush() method.
type Flusher interface {
	Flush()
}

// GetLevel get the log level string.
func (l *Logger) GetLevel() string {
	return l.origLogger.Level.String()
}

// SetLevel sets the log level. Valid levels are "debug", "info", "warn", "error", and "fatal".
func (l *Logger) SetLevel(level string) {
	lvl, err := log.ParseLevel(level)
	if err != nil {
		l.Fatal(fmt.Sprintf(`log level not valid: "%s"`, level))
	}
	l.origLogger.Level = lvl
}

// Flush writes buffered logs.
func (l *Logger) Flush() {
	if l.bufferedOut != nil {
		l.bufferedOut.Flush()
	}
}

// Debug logs a message with severity DEBUG.
func (l *Logger) Debug(format string, v ...interface{}) {
	l.output(l.origLogger.Debug, format, v...)
}

// Info logs a message with severity INFO.
func (l *Logger) Info(format string, v ...interface{}) {
	l.output(l.origLogger.Info, format, v...)
}

// Warn logs a message with severity WARN.
func (l *Logger) Warn(format string, v ...interface{}) {
	l.output(l.origLogger.Warn, format, v...)
}

// Error logs a message with severity ERROR.
func (l *Logger) Error(format string, v ...interface{}) {
	l.output(l.origLogger.Error, format, v...)
}

// Fatal logs a message with severity ERROR followed by a call to os.Exit().
func (l *Logger) Fatal(format string, v ...interface{}) {
	l.output(l.origLogger.Fatal, format, v...)
}

func (l *Logger) output(origin func(...interface{}), format string, v ...interface{}) {
	if len(v) > 0 {
		origin(fmt.Sprintf(format, v...))
	} else {
		origin(format)
	}
}

// CheckLevel checks whether the log level is valid.
func CheckLevel(level string) error {
	if _, err := log.ParseLevel(level); err != nil {
		return fmt.Errorf(`log level not valid: "%s"`, level)
	}
	return nil
}

// NewFileLogger creates a logger that write into files.
func NewFileLogger(filePath string, level ...string) (*Logger, error) {
	dir := path.Dir(filePath)
	if info, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf(`directory not exists: %s`, dir)
	} else if !info.IsDir() {
		return nil, fmt.Errorf(`path is not directory: %s`, dir)
	}

	out, err := reopen.NewFileWriter(filePath)
	if err != nil {
		return nil, err
	}

	c := make(chan os.Signal)
	go func() {
		for {
			select {
			case <-c:
				out.Reopen()
			}
		}
	}()
	signal.Notify(c, syscall.SIGHUP)

	return NewLogger(out, level...)
}

// NewBufferedFileLogger creates a logger that write into files with buffer.
func NewBufferedFileLogger(filePath string, level ...string) (*Logger, error) {
	dir := path.Dir(filePath)
	if info, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf(`directory not exists: %s`, dir)
	} else if !info.IsDir() {
		return nil, fmt.Errorf(`path is not directory: %s`, dir)
	}

	out, err := reopen.NewFileWriter(filePath)
	if err != nil {
		return nil, err
	}

	bufferedOut := reopen.NewBufferedFileWriter(out)

	c := make(chan os.Signal)
	go func() {
		for {
			select {
			case <-c:
				bufferedOut.Reopen()
			case <-time.After(10 * time.Second):
				bufferedOut.Flush()
			}
		}
	}()
	signal.Notify(c, syscall.SIGHUP)

	l, err := NewLogger(bufferedOut, level...)
	if err != nil {
		return nil, err
	}

	l.bufferedOut = bufferedOut

	return l, nil
}

// NewTerminalLogger creates a logger that write into terminal.
func NewTerminalLogger(level ...string) (*Logger, error) {
	return NewLogger(os.Stdout, level...)
}

// NewLogger creates a new logger for given out and level, and the level is
// optional.
func NewLogger(out io.Writer, level ...string) (*Logger, error) {
	if out == nil {
		return nil, errors.New(`must specify the output for logger`)
	}
	l := &Logger{
		origLogger: &log.Logger{
			Out:       out,
			Formatter: NewLogFormatter(),
			Hooks:     log.LevelHooks{},
			Level:     log.WarnLevel,
		},
		out: out,
	}

	if len(level) == 1 {
		if err := CheckLevel(level[0]); err != nil {
			return nil, err
		}
		l.SetLevel(level[0])
	}

	return l, nil
}
