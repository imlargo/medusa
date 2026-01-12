// Package logger provides structured logging capabilities using Uber's Zap library.
//
// The Logger wraps zap.Logger to provide high-performance, structured, and leveled logging
// suitable for production environments. It supports multiple log levels (Debug, Info, Warn, Error)
// and automatically adds structured fields to log entries.
//
// Example usage:
//
//	log := logger.NewLogger()
//	defer log.Sync() // Flush any buffered log entries
//
//	log.Info("Server started",
//	    zap.String("host", "localhost"),
//	    zap.Int("port", 8080),
//	)
//
//	log.Error("Failed to connect to database",
//	    zap.Error(err),
//	    zap.String("database", "postgres"),
//	)
//
// For more information on zap fields and usage, see: https://pkg.go.dev/go.uber.org/zap
package logger

import "go.uber.org/zap"

// Logger wraps zap.Logger to provide structured logging capabilities.
// It embeds *zap.Logger, making all zap methods directly available.
//
// The logger is safe for concurrent use by multiple goroutines.
type Logger struct {
	*zap.Logger
}

// NewLogger creates a new production-ready logger with sensible defaults.
// The logger uses JSON encoding, logs at Info level and above, and includes
// caller information for warnings and errors.
//
// The returned logger should be synchronized before application exit to ensure
// all buffered log entries are flushed:
//
//	log := logger.NewLogger()
//	defer log.Sync()
//
// If you need a development logger with human-readable output, use zap.NewDevelopment() directly.
func NewLogger() *Logger {
	logger, _ := zap.NewProduction()
	return &Logger{Logger: logger}
}
