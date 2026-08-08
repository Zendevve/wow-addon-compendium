// Package logger provides a small, concurrency-safe logger that keeps
// a bounded ring buffer of entries in memory and optionally mirrors
// them to a file. Entries are readable in memory, and Export writes
// the current buffer to disk.
package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Level of a log entry.
type Level string

const (
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

// Entry is one log line.
type Entry struct {
	Time    time.Time
	Level   Level
	Message string
}

// Logger is a thread-safe ring buffer.
type Logger struct {
	mu   sync.Mutex
	ring []Entry
	next int
	size int
	file *os.File
}

// New creates a logger holding up to capacity entries.
func New(capacity int) *Logger {
	if capacity <= 0 {
		capacity = 500
	}
	return &Logger{ring: make([]Entry, capacity), size: capacity}
}

// AttachFile mirrors future entries to the given path (appended).
func (l *Logger) AttachFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	l.mu.Lock()
	l.file = f
	l.mu.Unlock()
	return nil
}

// Close closes the attached file, if any.
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

func (l *Logger) log(level Level, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := Entry{Time: time.Now(), Level: level, Message: msg}
	l.ring[l.next] = e
	l.next = (l.next + 1) % l.size
	if l.file != nil {
		fmt.Fprintf(l.file, "%s %-5s %s\n", e.Time.Format("2006-01-02 15:04:05"), level, msg)
	}
}

// Info logs an informational entry.
func (l *Logger) Info(msg string) { l.log(LevelInfo, msg) }

// Infof logs a formatted informational entry.
func (l *Logger) Infof(format string, args ...any) { l.log(LevelInfo, fmt.Sprintf(format, args...)) }

// Warn logs a warning entry.
func (l *Logger) Warn(msg string) { l.log(LevelWarn, msg) }

// Error logs an error entry.
func (l *Logger) Error(msg string) { l.log(LevelError, msg) }

// Errorf logs a formatted error entry.
func (l *Logger) Errorf(format string, args ...any) { l.log(LevelError, fmt.Sprintf(format, args...)) }

// Entries returns a snapshot of the ring buffer in chronological order.
func (l *Logger) Entries() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Entry, 0, l.size)
	for i := 0; i < l.size; i++ {
		e := l.ring[(l.next+i)%l.size]
		if e.Time.IsZero() {
			continue
		}
		out = append(out, e)
	}
	return out
}

// Text renders all entries as plain text lines.
func (l *Logger) Text() string {
	entries := l.Entries()
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%s %-5s %s\n", e.Time.Format("2006-01-02 15:04:05"), e.Level, e.Message)
	}
	return b.String()
}

// Export writes the current buffer to a file and returns its path.
func (l *Logger) Export(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("wowfix-%s.log", time.Now().Format("20060102-150405")))
	if err := os.WriteFile(path, []byte(l.Text()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
