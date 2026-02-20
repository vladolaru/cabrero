package daemon

import (
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	defaultMaxSize  = 5 * 1024 * 1024 // 5 MB
	maxRotatedFiles = 2               // .1 and .2
)

// Logger writes timestamped entries to a file with size-based rotation.
type Logger struct {
	path    string
	maxSize int64
	file    *os.File
	mu      sync.Mutex
}

// NewLogger creates a logger writing to path, rotating when the file exceeds maxSize.
// Pass 0 for maxSize to use the default (5 MB).
func NewLogger(path string, maxSize int64) (*Logger, error) {
	if maxSize <= 0 {
		maxSize = defaultMaxSize
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}

	return &Logger{path: path, maxSize: maxSize, file: f}, nil
}

// Info logs an informational message.
func (l *Logger) Info(format string, args ...any) {
	l.write("INFO", format, args...)
}

// Error logs an error message.
func (l *Logger) Error(format string, args ...any) {
	l.write("ERROR", format, args...)
}

// Close flushes and closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

func (l *Logger) write(level, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s [%s] %s\n", time.Now().Format("2006-01-02T15:04:05"), level, msg)

	if l.file == nil {
		return
	}

	_, _ = l.file.WriteString(line)
	_ = l.file.Sync()

	l.rotateIfNeeded()
}

func (l *Logger) rotateIfNeeded() {
	info, err := l.file.Stat()
	if err != nil || info.Size() < l.maxSize {
		return
	}

	l.file.Close()

	// Shift existing rotated files: .2 is dropped, .1 → .2
	for i := maxRotatedFiles; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", l.path, i)
		if i == maxRotatedFiles {
			os.Remove(src)
		} else {
			dst := fmt.Sprintf("%s.%d", l.path, i+1)
			os.Rename(src, dst)
		}
	}

	// Current → .1
	os.Rename(l.path, l.path+".1")

	// Open fresh file.
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		l.file = nil
		return
	}
	l.file = f
}
