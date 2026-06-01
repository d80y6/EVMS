package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dam-vms/dam/pkg/common"
)

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

type ThumbnailConfig struct {
	Port            string
	RecordingsRoot  string
	CacheRoot       string
	MetricsAddr     string
	RequestTimeout  time.Duration
	MinInterval     int
	DefaultInterval int
}

func DefaultThumbnailConfig() *ThumbnailConfig {
	return &ThumbnailConfig{
		Port:            common.GetEnv("THUMBNAILS_PORT", ":8089"),
		RecordingsRoot:  common.GetEnv("RECORDINGS_ROOT", "/recordings"),
		CacheRoot:       common.GetEnv("THUMBNAIL_CACHE", "/cache/thumbnails"),
		MetricsAddr:     common.GetEnv("METRICS_ADDR", ":2112"),
		RequestTimeout:  30 * time.Second,
		MinInterval:     10,
		DefaultInterval: 60,
	}
}

type ThumbnailService struct {
	config        *ThumbnailConfig
	logger        *slog.Logger
	server        *http.Server
	mu            sync.Mutex
	healthHandler *common.HealthHandler
}

func NewThumbnailService(config *ThumbnailConfig, logger *slog.Logger) (*ThumbnailService, error) {
	if err := os.MkdirAll(config.CacheRoot, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}
	return &ThumbnailService{
		config:        config,
		logger:        logger,
		healthHandler: common.NewHealthHandler(),
	}, nil
}

func (s *ThumbnailService) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/thumbnails/timeline", s.handleTimeline)
	mux.HandleFunc("/thumbnails/image/", s.handleImage)
	mux.HandleFunc("/health", s.healthHandler.Liveness)
	mux.HandleFunc("/ready", s.healthHandler.Readiness)

	s.server = &http.Server{
		Addr:         s.config.Port,
		Handler:      common.RecoveryMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	s.logger.Info("Thumbnails Service started", "address", s.config.Port, "cache", s.config.CacheRoot)
	return s.server.ListenAndServe()
}

func (s *ThumbnailService) Shutdown(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}



type timelineRequest struct {
	CameraID string `json:"camera_id"`
	Start    string `json:"start"`
	End      string `json:"end"`
	Interval int    `json:"interval"`
}

type timelineResponse struct {
	Thumbnails []thumbnailItem `json:"thumbnails"`
}

type thumbnailItem struct {
	Timestamp string `json:"timestamp"`
	URL       string `json:"url"`
}

func (s *ThumbnailService) handleTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cameraID := r.URL.Query().Get("camera_id")
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	intervalStr := r.URL.Query().Get("interval")

	if cameraID == "" || startStr == "" || endStr == "" {
		jsonError(w, "camera_id, start, and end query parameters required", http.StatusBadRequest)
		return
	}

	interval := s.config.DefaultInterval
	if intervalStr != "" {
		if v, err := strconv.Atoi(intervalStr); err == nil && v >= s.config.MinInterval {
			interval = v
		}
	}

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		start, err = time.Parse(time.RFC3339Nano, startStr)
		if err != nil {
			jsonError(w, "invalid start time format (use ISO8601/RFC3339)", http.StatusBadRequest)
			return
		}
	}

	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		end, err = time.Parse(time.RFC3339Nano, endStr)
		if err != nil {
			jsonError(w, "invalid end time format (use ISO8601/RFC3339)", http.StatusBadRequest)
			return
		}
	}

	s.logger.Info("Timeline request", "camera", cameraID, "start", start, "end", end, "interval", interval)

	var thumbnails []thumbnailItem
	current := start
	for current.Before(end) || current.Equal(end) {
		ts := current
		cachePath := s.cachePath(cameraID, ts)

		url := fmt.Sprintf("/thumbnails/image/%s/%s.jpg", cameraID, ts.Format("20060102_150405"))

		if _, err := os.Stat(cachePath); os.IsNotExist(err) {
			go s.generateThumbnail(cameraID, ts)
			thumbnails = append(thumbnails, thumbnailItem{
				Timestamp: ts.Format(time.RFC3339),
				URL:       "",
			})
		} else {
			thumbnails = append(thumbnails, thumbnailItem{
				Timestamp: ts.Format(time.RFC3339),
				URL:       url,
			})
		}

		current = current.Add(time.Duration(interval) * time.Second)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(timelineResponse{Thumbnails: thumbnails})
}

func (s *ThumbnailService) handleImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/thumbnails/image/")

	re := regexp.MustCompile(`^([^/]+)/(\d{8}_\d{6})\.jpg$`)
	matches := re.FindStringSubmatch(path)
	if matches == nil {
		jsonError(w, "invalid image path: /thumbnails/image/{camera_id}/{timestamp}.jpg", http.StatusBadRequest)
		return
	}

	cameraID := matches[1]
	timestampStr := matches[2]

	ts, err := time.Parse("20060102_150405", timestampStr)
	if err != nil {
		jsonError(w, "invalid timestamp format", http.StatusBadRequest)
		return
	}

	cachePath := s.cachePath(cameraID, ts)

	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		if err := s.generateThumbnail(cameraID, ts); err != nil {
			s.logger.Error("Failed to generate thumbnail", "camera", cameraID, "timestamp", ts, "error", err)
			jsonError(w, "thumbnail not available", http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeFile(w, r, cachePath)
}

func (s *ThumbnailService) cachePath(cameraID string, ts time.Time) string {
	return filepath.Join(s.config.CacheRoot, cameraID, ts.Format("20060102_150405")+".jpg")
}

func (s *ThumbnailService) findRecording(cameraID string, ts time.Time) string {
	pattern := filepath.Join(s.config.RecordingsRoot, cameraID, "*.mp4")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return ""
	}

	var bestMatch string
	var bestDiff time.Duration

	for _, match := range matches {
		base := filepath.Base(match)
		base = strings.TrimSuffix(base, ".mp4")
		parts := strings.Split(base, "_")
		if len(parts) < 2 {
			continue
		}
		start, err := time.Parse("20060102_150405", parts[0])
		if err != nil {
			continue
		}
		end, err := time.Parse("20060102_150405", parts[len(parts)-1])
		if err != nil {
			end = start.Add(time.Hour)
		}

		if (ts.Equal(start) || ts.After(start)) && ts.Before(end) {
			diff := ts.Sub(start)
			if bestMatch == "" || diff < bestDiff {
				bestMatch = match
				bestDiff = diff
			}
		}
	}

	return bestMatch
}

func (s *ThumbnailService) generateThumbnail(cameraID string, ts time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cachePath := s.cachePath(cameraID, ts)

	if _, err := os.Stat(cachePath); err == nil {
		return nil
	}

	recordingPath := s.findRecording(cameraID, ts)
	if recordingPath == "" {
		return fmt.Errorf("no recording found for camera %s at %s", cameraID, ts.Format(time.RFC3339))
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	seekStr := ts.Format("15:04:05.000")
	if ts.Year() > 2000 {
		seekStr = ts.Format("2006-01-02 15:04:05.000")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-ss", seekStr,
		"-i", recordingPath,
		"-vframes", "1",
		"-s", "320x180",
		"-q:v", "10",
		"-f", "image2",
		cachePath,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("thumbnail generation timed out for %s at %s", cameraID, seekStr)
		}
		return fmt.Errorf("ffmpeg failed: %w, stderr: %s", err, stderr.String())
	}

	s.logger.Info("Generated thumbnail", "camera", cameraID, "timestamp", ts.Format(time.RFC3339), "path", cachePath)
	return nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultThumbnailConfig()
	common.StartMetricsServer(config.MetricsAddr)

	service, err := NewThumbnailService(config, logger)
	if err != nil {
		logger.Error("Failed to initialize thumbnails service", "error", err)
		os.Exit(1)
	}

	go func() {
		if err := service.Start(); err != nil && err != http.ErrServerClosed {
			logger.Error("Thumbnails service failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("Shutting down Thumbnails Service...")

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := service.Shutdown(shutdownCtx); err != nil {
		logger.Error("Error during shutdown", "error", err)
	}
}
