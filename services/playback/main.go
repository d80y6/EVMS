package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dam-vms/dam/pkg/common"
)

// PlaybackConfig holds configuration for the playback service
type PlaybackConfig struct {
	RecordingsRoot string
	Port           string
}

// DefaultPlaybackConfig returns default configuration values
func DefaultPlaybackConfig() *PlaybackConfig {
	return &PlaybackConfig{
		RecordingsRoot: getEnv("RECORDINGS_ROOT", "/recordings"),
		Port:           getEnv("PLAYBACK_PORT", ":8086"),
	}
}

// PlaybackService handles video playback requests
type PlaybackService struct {
	config     *PlaybackConfig
	logger     *slog.Logger
	recordings string // absolute path to recordings root
}

// NewPlaybackService creates a new playback service instance
func NewPlaybackService(config *PlaybackConfig, logger *slog.Logger) (*PlaybackService, error) {
	absRoot, err := filepath.Abs(config.RecordingsRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for recordings root: %w", err)
	}

	return &PlaybackService{
		config:     config,
		logger:     logger,
		recordings: absRoot,
	}, nil
}

// Start begins the HTTP server
func (s *PlaybackService) Start() error {
	playbackHandler := http.HandlerFunc(s.handlePlaybackRequest)
	http.HandleFunc("/playback/", common.JWTAuthMiddleware(playbackHandler))

	s.logger.Info("Hardened Playback Service started", "address", s.config.Port, "root", s.recordings)

	server := &http.Server{
		Addr:         s.config.Port,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	return server.ListenAndServe()
}

// handlePlaybackRequest serves recorded mp4 files with security hardening
func (s *PlaybackService) handlePlaybackRequest(w http.ResponseWriter, r *http.Request) {
	// 1. Path Sanitization & Traversal Prevention
	relPath := r.URL.Path[len("/playback/"):]
	fullPath := filepath.Join(s.recordings, relPath)

	// Ensure the resulting path is still within recordings root
	finalPath, err := filepath.Abs(fullPath)
	if err != nil || !strings.HasPrefix(finalPath, s.recordings) {
		s.logger.Warn("Blocked traversal attempt", "path", relPath, "remote_addr", r.RemoteAddr)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// 2. Check if file exists and is not a directory
	info, err := os.Stat(finalPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		s.logger.Error("Failed to stat file", "path", finalPath, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if info.IsDir() {
		s.logger.Warn("Directory access attempted", "path", finalPath, "remote_addr", r.RemoteAddr)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	s.logger.Info("Playback request", "path", finalPath, "remote_addr", r.RemoteAddr)
	http.ServeFile(w, r, finalPath)
}

// Shutdown gracefully stops the HTTP server
func (s *PlaybackService) Shutdown(ctx context.Context) error {
	server := &http.Server{Addr: s.config.Port}
	return server.Shutdown(ctx)
}

// getEnv retrieves environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	config := DefaultPlaybackConfig()

	service, err := NewPlaybackService(config, logger)
	if err != nil {
		logger.Error("Failed to initialize playback service", "error", err)
		os.Exit(1)
	}

	if err := service.Start(); err != nil {
		logger.Error("Playback service failed", "error", err)
		os.Exit(1)
	}
}
