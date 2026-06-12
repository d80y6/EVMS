package common

import (
	"log/slog"
	"os"
	"strings"
)

// NewLogger creates a JSON slog.Logger with configurable level from env var.
// Env var: LOG_LEVEL (default "info").
// Valid values: debug, info, warn, error.
func NewLogger(serviceName string) *slog.Logger {
	level := parseLogLevel(os.Getenv("LOG_LEVEL"))
	opts := &slog.HandlerOptions{Level: level}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, opts))
	logger = logger.With("service", serviceName)
	return logger
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
