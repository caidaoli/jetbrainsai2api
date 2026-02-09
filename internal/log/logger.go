package log

import (
	"io"
	"log"
	"os"
	"sync"

	"jetbrainsai2api/internal/core"
)

// LogLevel defines the severity level for log messages.
type LogLevel int

// Log level constants.
const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

// AppLogger is the application logger implementation.
type AppLogger struct {
	logger     *log.Logger
	debug      bool
	fileHandle *os.File
	mu         sync.RWMutex
}

// NewAppLoggerWithConfig creates a logger instance with configuration.
func NewAppLoggerWithConfig(output io.Writer, debugMode bool) *AppLogger {
	return &AppLogger{
		logger:     log.New(output, "", log.LstdFlags),
		debug:      debugMode,
		fileHandle: nil,
	}
}

// Debug logs a message at DEBUG level.
func (l *AppLogger) Debug(format string, args ...any) {
	if l != nil && l.debug {
		l.logger.Printf("[DEBUG] "+format, args...)
	}
}

// Info logs a message at INFO level.
func (l *AppLogger) Info(format string, args ...any) {
	if l != nil {
		l.logger.Printf("[INFO] "+format, args...)
	}
}

// Warn logs a message at WARN level.
func (l *AppLogger) Warn(format string, args ...any) {
	if l != nil {
		l.logger.Printf("[WARN] "+format, args...)
	}
}

// Error logs a message at ERROR level.
func (l *AppLogger) Error(format string, args ...any) {
	if l != nil {
		l.logger.Printf("[ERROR] "+format, args...)
	}
}

// Fatal logs a message at FATAL level and terminates the process.
func (l *AppLogger) Fatal(format string, args ...any) {
	if l != nil {
		l.logger.Fatalf("[FATAL] "+format, args...)
	} else {
		log.Fatalf("[FATAL] "+format, args...)
	}
}

// Close safely closes log file handle.
func (l *AppLogger) Close() error {
	if l == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.fileHandle != nil {
		err := l.fileHandle.Close()
		l.fileHandle = nil
		return err
	}
	return nil
}

// containsPathTraversal checks if path contains path traversal characters.
func containsPathTraversal(path string) bool {
	dangerousPatterns := []string{
		"..", "./", "../", "..\\", ".\\",
	}

	for _, pattern := range dangerousPatterns {
		if len(path) >= len(pattern) {
			for i := 0; i <= len(path)-len(pattern); i++ {
				if path[i:i+len(pattern)] == pattern {
					return true
				}
			}
		}
	}

	return false
}

// createDebugFileOutput creates debug file output, falls back gracefully on failure.
func createDebugFileOutput() (io.Writer, *os.File) {
	debugFile := os.Getenv("DEBUG_FILE")
	if debugFile == "" {
		return os.Stdout, nil
	}

	if len(debugFile) > core.MaxDebugFilePathLength {
		log.Printf("[WARN] DEBUG_FILE path too long, falling back to stdout")
		return os.Stdout, nil
	}

	cleanPath := os.Getenv("DEBUG_FILE")
	if len(cleanPath) > 0 {
		if containsPathTraversal(cleanPath) {
			log.Printf("[WARN] DEBUG_FILE contains path traversal characters, falling back to stdout")
			return os.Stdout, nil
		}
	}

	//nolint:gosec // G304: debugFile from env var, validated by containsPathTraversal
	file, err := os.OpenFile(debugFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, core.FilePermissionReadWrite)
	if err != nil {
		log.Printf("[WARN] Failed to open DEBUG_FILE '%s': %v, falling back to stdout", debugFile, err)
		return os.Stdout, nil
	}

	return file, file
}

// IsDebug returns whether the app is running in debug mode.
func IsDebug() bool {
	return os.Getenv("GIN_MODE") == "debug"
}

// CreateLogger creates a logger instance (for dependency injection).
func CreateLogger() core.Logger {
	debugMode := IsDebug()
	output, fileHandle := createDebugFileOutput()

	logger := &AppLogger{
		logger:     log.New(output, "", log.LstdFlags),
		debug:      debugMode,
		fileHandle: fileHandle,
	}

	return logger
}
