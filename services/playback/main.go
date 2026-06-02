package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dam-vms/dam/pkg/common"
)

type PlaybackConfig struct {
	RecordingsRoot string
	Port           string
}

func DefaultPlaybackConfig() *PlaybackConfig {
	return &PlaybackConfig{
		RecordingsRoot: common.GetEnv("RECORDINGS_ROOT", "/recordings"),
		Port:           common.GetEnv("PLAYBACK_PORT", ":8086"),
	}
}

type PlaybackService struct {
	config        *PlaybackConfig
	logger        *slog.Logger
	recordings    string
	server        *http.Server
	healthHandler *common.HealthHandler
}

func NewPlaybackService(config *PlaybackConfig, logger *slog.Logger) (*PlaybackService, error) {
	if logger == nil {
		logger = slog.Default()
	}
	absRoot, err := filepath.Abs(config.RecordingsRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for recordings root: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("recordings root does not exist: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("recordings root is not a directory: %s", absRoot)
	}
	return &PlaybackService{
		config:        config,
		logger:        logger,
		recordings:    absRoot,
		healthHandler: common.NewHealthHandler(),
	}, nil
}

func (s *PlaybackService) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/playback/", common.JWTAuthMiddleware(http.HandlerFunc(s.handlePlaybackRequest)))
	mux.HandleFunc("/health", s.healthHandler.Liveness)
	mux.HandleFunc("/ready", s.healthHandler.Readiness)

	s.server = &http.Server{
		Addr:         s.config.Port,
		Handler:      common.RecoveryMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	s.logger.Info("Playback Service started", "address", s.config.Port, "root", s.recordings)
	return s.server.ListenAndServe()
}

func (s *PlaybackService) Shutdown(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}



func (s *PlaybackService) handlePlaybackRequest(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/playback/")
	relPath = strings.TrimLeft(filepath.Clean(relPath), "/")
	if relPath == "" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(s.recordings, relPath)

	evalPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		s.logger.Error("Failed to resolve path", "path", fullPath, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	rel, err := filepath.Rel(s.recordings, evalPath)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		s.logger.Warn("Blocked traversal attempt", "path", relPath, "resolved", evalPath, "remote_addr", r.RemoteAddr)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	info, err := os.Stat(evalPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		s.logger.Error("Failed to stat file", "path", evalPath, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if info.IsDir() {
		s.logger.Warn("Directory access attempted", "path", relPath, "remote_addr", r.RemoteAddr)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	s.logger.Info("Playback request", "path", evalPath, "remote_addr", r.RemoteAddr)
	http.ServeFile(w, r, evalPath)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := common.InitTelemetry("playback"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultPlaybackConfig()

	common.StartMetricsServer(common.GetEnv("METRICS_ADDR", ":2112"))
	common.StartResourceMonitor(ctx)

	service, err := NewPlaybackService(config, logger)
	if err != nil {
		logger.Error("Failed to initialize playback service", "error", err)
		os.Exit(1)
	}

	go func() {
		if err := service.Start(); err != nil && err != http.ErrServerClosed {
			logger.Error("Playback service failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("Shutting down Playback Service...")

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := service.Shutdown(shutdownCtx); err != nil {
		logger.Error("Error during shutdown", "error", err)
	}
}
