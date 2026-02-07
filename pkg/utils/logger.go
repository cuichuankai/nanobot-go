package utils

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// RotatableLogger writes to a file and rotates it when it reaches a certain size.
type RotatableLogger struct {
	Filename   string
	MaxSize    int64 // bytes
	MaxBackups int
	file       *os.File
	mu         sync.Mutex
}

// NewRotatableLogger creates a new RotatableLogger.
func NewRotatableLogger(filename string, maxSize int64, maxBackups int) *RotatableLogger {
	return &RotatableLogger{
		Filename:   filename,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
	}
}

func (l *RotatableLogger) open() error {
	file, err := os.OpenFile(l.Filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	l.file = file
	return nil
}

func (l *RotatableLogger) close() error {
	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}

func (l *RotatableLogger) rotate() error {
	if err := l.close(); err != nil {
		return err
	}

	for i := l.MaxBackups - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", l.Filename, i)
		newPath := fmt.Sprintf("%s.%d", l.Filename, i+1)
		os.Rename(oldPath, newPath)
	}

	if l.MaxBackups > 0 {
		os.Rename(l.Filename, fmt.Sprintf("%s.1", l.Filename))
	}

	return l.open()
}

func (l *RotatableLogger) Write(p []byte) (n int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		if err := l.open(); err != nil {
			// Fallback to stderr if file open fails
			return os.Stderr.Write(p)
		}
	}

	info, err := l.file.Stat()
	if err == nil && info.Size() > l.MaxSize {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}

	return l.file.Write(p)
}

// SetupLogger configures the global logger to use the rotatable logger.
func SetupLogger(logDir string) {
	os.MkdirAll(logDir, 0755)
	logFile := filepath.Join(logDir, "nanobot.log")

	// 10MB limit, 5 backups
	logger := NewRotatableLogger(logFile, 10*1024*1024, 5)

	// Write to both file and stderr
	mw := io.MultiWriter(os.Stderr, logger)
	log.SetOutput(mw)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}
