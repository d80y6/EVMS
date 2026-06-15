package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
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
	db            *sqlx.DB
}

func NewPlaybackService(config *PlaybackConfig, logger *slog.Logger, db *sqlx.DB) (*PlaybackService, error) {
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
		db:            db,
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

	// Authorize: verify camera belongs to user's tenant
	if s.db != nil {
		parts := strings.SplitN(relPath, "/", 2)
		if len(parts) > 0 && parts[0] != "" {
			tenantID := r.Header.Get("X-Tenant-ID")
			if tenantID != "" {
				var count int
				err := s.db.Get(&count,
					`SELECT COUNT(*) FROM cameras c
					 JOIN sites s ON c.site_id = s.id
					 WHERE c.id = $1 AND s.tenant_id = $2`,
					parts[0], tenantID)
				if err != nil || count == 0 {
					s.logger.Warn("Blocked unauthorized playback access",
						"camera", parts[0], "remote_addr", r.RemoteAddr)
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
			}
		}
	}

	// Audio playback support
	if strings.HasPrefix(relPath, "audio/") {
		s.handleAudioPlayback(w, r, relPath)
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

	// Check for audio-only range request
	if r.URL.Query().Get("audio_only") == "true" || strings.HasSuffix(relPath, ".aac") || strings.HasSuffix(relPath, ".wav") {
		w.Header().Set("Content-Type", s.audioContentType(relPath))
	}

	// Integrity check for MP4 files before serving
	if strings.HasSuffix(relPath, ".mp4") && info.Size() > 8 {
		if err := validateMP4(evalPath, s.logger); err != nil {
			s.logger.Warn("MP4 integrity check failed, serving anyway", "path", evalPath, "error", err)
		}
	}

	s.logger.Info("Playback request", "path", evalPath, "remote_addr", r.RemoteAddr)
	http.ServeFile(w, r, evalPath)
}

func validateMP4(path string, logger *slog.Logger) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	header := make([]byte, 16)
	if _, err := io.ReadFull(f, header); err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	if string(header[4:8]) != "ftyp" {
		return fmt.Errorf("missing ftyp box")
	}

	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() < 1024 {
		return fmt.Errorf("file too small: %d bytes", info.Size())
	}

	return nil
}

func (s *PlaybackService) handleAudioPlayback(w http.ResponseWriter, r *http.Request, relPath string) {
	audioPath := strings.TrimPrefix(relPath, "audio/")
	fullPath := filepath.Join(s.recordings, audioPath)

	evalPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		s.logger.Error("Failed to resolve audio path", "path", fullPath, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	rel, err := filepath.Rel(s.recordings, evalPath)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		s.logger.Warn("Blocked audio path traversal", "path", relPath, "remote_addr", r.RemoteAddr)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", s.audioContentType(evalPath))
	w.Header().Set("Accept-Ranges", "bytes")
	s.logger.Info("Audio playback request", "path", evalPath, "remote_addr", r.RemoteAddr)
	http.ServeFile(w, r, evalPath)
}

func (s *PlaybackService) audioContentType(path string) string {
	switch {
	case strings.HasSuffix(path, ".aac"):
		return "audio/aac"
	case strings.HasSuffix(path, ".wav"):
		return "audio/wav"
	case strings.HasSuffix(path, ".mp3"):
		return "audio/mpeg"
	case strings.HasSuffix(path, ".opus"):
		return "audio/opus"
	case strings.HasSuffix(path, ".ogg"):
		return "audio/ogg"
	default:
		return "audio/mpeg"
	}
}

func main() {
	logger := common.NewLogger("playback")
	slog.SetDefault(logger)

	common.CheckJWTSecret()

	if err := common.InitTelemetry("playback"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultPlaybackConfig()

	common.StartMetricsServer(common.GetEnv("METRICS_ADDR", ":2112"))
	common.StartResourceMonitor(ctx)

	var db *sqlx.DB
	if dbURL := os.Getenv("DB_URL"); dbURL != "" {
		cb := common.NewDBCircuitBreaker("playback")
		dbCtx, dbCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer dbCancel()
		var err error
		db, err = common.ConnectDBWithCircuitBreaker(dbCtx, "postgres", dbURL, cb)
		if err != nil {
			logger.Warn("Failed to connect to database, playback auth disabled", "error", err)
		} else {
			logger.Info("Connected to database")
			healthHandler := common.NewHealthHandler()
			healthHandler.AddDBChecker(db.DB, "postgres")
		}
	}

	service, err := NewPlaybackService(config, logger, db)
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
