package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
)

// RecorderConfig holds configuration for the recorder service
type RecorderConfig struct {
	DBURL           string
	NATSURL         string
	MetricsAddr     string
	RetentionDays   int
	CleanupInterval time.Duration
}

// DefaultRecorderConfig returns a configuration with sensible defaults
func DefaultRecorderConfig() RecorderConfig {
	return RecorderConfig{
		MetricsAddr:     ":2112",
		NATSURL:         "nats://nats:4222",
		RetentionDays:   7,
		CleanupInterval: 1 * time.Hour,
	}
}

// Validate checks if the configuration is valid
func (c *RecorderConfig) Validate() error {
	if c.DBURL == "" {
		return errors.New("DB_URL environment variable is required")
	}
	if c.NATSURL == "" {
		return errors.New("NATS_URL environment variable is required")
	}
	return nil
}

// RecordingSegment represents a recorded video segment
type RecordingSegment struct {
	CameraID  string    `db:"camera_id"`
	StartTime time.Time `db:"start_time"`
	EndTime   time.Time `db:"end_time"`
	FilePath  string    `db:"file_path"`
	FileSize  int64     `db:"file_size"`
}

// RecordingEvent represents a NATS event for a new recording
type RecordingEvent struct {
	CameraID string `json:"camera_id"`
	Path     string `json:"path"`
}

// Recorder handles recording indexing and retention
type Recorder struct {
	db     *sqlx.DB
	logger *slog.Logger
	config RecorderConfig
	sub    *nats.Subscription
}

// NewRecorder creates a new recorder instance
func NewRecorder(ctx context.Context, config RecorderConfig, logger *slog.Logger) (*Recorder, error) {
	db, err := sqlx.ConnectContext(ctx, "postgres", config.DBURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &Recorder{
		db:     db,
		logger: logger,
		config: config,
	}, nil
}

// Close gracefully shuts down the recorder
func (r *Recorder) Close() error {
	var errs []error
	if r.sub != nil {
		if err := r.sub.Unsubscribe(); err != nil {
			errs = append(errs, err)
		}
	}
	if r.db != nil {
		if err := r.db.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors during recorder shutdown: %v", errs)
	}
	return nil
}

// IndexSegment stores a recording segment in the database
func (r *Recorder) IndexSegment(ctx context.Context, seg RecordingSegment) error {
	query := `INSERT INTO recordings (camera_id, start_time, end_time, file_path, file_size)
              VALUES (:camera_id, :start_time, :end_time, :file_path, :file_size)`
	_, err := r.db.NamedExecContext(ctx, query, seg)
	if err != nil {
		r.logger.Error("Failed to index segment", "error", err, "camera_id", seg.CameraID)
		return fmt.Errorf("failed to index segment: %w", err)
	}
	r.logger.Info("Indexed recording segment",
		"camera_id", seg.CameraID,
		"path", seg.FilePath,
		"size", seg.FileSize)
	common.RecordingsIndexed.WithLabelValues(seg.CameraID).Inc()
	return nil
}

// Listen subscribes to recording events and indexes them
func (r *Recorder) Listen(ctx context.Context, nc *nats.Conn) error {
	var err error
	r.sub, err = nc.QueueSubscribe("camera.*.recordings.new", "recorder", func(msg *nats.Msg) {
		var event RecordingEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			r.logger.Debug("Failed to unmarshal recording event", "error", err)
			return
		}

		segment, err := r.processRecordingEvent(ctx, event)
		if err != nil {
			r.logger.Error("Failed to process recording event", "error", err, "path", event.Path)
			return
		}

		if err := r.IndexSegment(ctx, segment); err != nil {
			r.logger.Error("Failed to index segment", "error", err)
		}
	})
	if err != nil {
		return err
	}
	r.sub.SetPendingLimits(1024, 64*1024*1024)
	return nil
}

// processRecordingEvent processes a recording event and returns a segment
func (r *Recorder) processRecordingEvent(ctx context.Context, event RecordingEvent) (RecordingSegment, error) {
	// Wait for file to be finalized by FFmpeg
	maxRetries := 5
	var info os.FileInfo
	var err error

	for i := 0; i < maxRetries; i++ {
		info, err = os.Stat(event.Path)
		if err == nil && info.Size() > 0 {
			break
		}
		select {
		case <-ctx.Done():
			return RecordingSegment{}, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	if err != nil {
		return RecordingSegment{}, fmt.Errorf("could not stat recording file: %w", err)
	}

	filename := filepath.Base(event.Path)
	timeStr := strings.TrimSuffix(filename, ".mp4")

	// Format from ingest: 20240101_120000.mp4
	startTime, err := time.Parse("20060102_150405", timeStr)
	if err != nil {
		r.logger.Debug("Could not parse timestamp from filename, using current time",
			"filename", filename, "error", err)
		startTime = time.Now()
	}

	return RecordingSegment{
		CameraID:  event.CameraID,
		StartTime: startTime,
		EndTime:   startTime.Add(60 * time.Second),
		FilePath:  event.Path,
		FileSize:  info.Size(),
	}, nil
}

// StartRetentionWorker starts a background worker for retention cleanup
func (r *Recorder) StartRetentionWorker(ctx context.Context) {
	ticker := time.NewTicker(r.config.CleanupInterval)
	defer ticker.Stop()

	r.logger.Info("Starting retention worker",
		"interval", r.config.CleanupInterval,
		"retention_days", r.config.RetentionDays)

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("Stopping retention worker")
			return
		case <-ticker.C:
			r.runRetentionCleanup(ctx)
		}
	}
}

// runRetentionCleanup removes recordings older than the retention period
func (r *Recorder) runRetentionCleanup(ctx context.Context) {
	cutoff := time.Now().AddDate(0, 0, -r.config.RetentionDays)
	r.logger.Info("Running retention cleanup", "cutoff", cutoff)

	var segments []RecordingSegment
	err := r.db.SelectContext(ctx, &segments,
		"SELECT camera_id, file_path FROM recordings WHERE start_time < $1", cutoff)
	if err != nil {
		r.logger.Error("Failed to fetch expired segments", "error", err)
		return
	}

	deletedCount := 0
	for _, seg := range segments {
		if err := os.Remove(seg.FilePath); err != nil && !os.IsNotExist(err) {
			r.logger.Error("Failed to delete recording file", "path", seg.FilePath, "error", err)
			continue
		}

		if _, err := r.db.ExecContext(ctx,
			"DELETE FROM recordings WHERE file_path = $1", seg.FilePath); err != nil {
			r.logger.Error("Failed to delete recording record", "path", seg.FilePath, "error", err)
			continue
		}
		deletedCount++
	}

	r.logger.Info("Retention cleanup finished", "deleted_count", deletedCount)
}

// RecorderService manages the recorder service lifecycle
type RecorderService struct {
	config   RecorderConfig
	logger   *slog.Logger
	recorder *Recorder
	nc       *nats.Conn
}

// NewRecorderService creates a new recorder service
func NewRecorderService(config RecorderConfig, logger *slog.Logger) (*RecorderService, error) {
	nc, err := nats.Connect(config.NATSURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	return &RecorderService{
		config: config,
		logger: logger,
		nc:     nc,
	}, nil
}

// Close gracefully shuts down the service
func (s *RecorderService) Close() error {
	var errs []error
	if s.recorder != nil {
		if err := s.recorder.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.nc != nil {
		s.nc.Close()
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errs)
	}
	return nil
}

// Start starts the recorder service
func (s *RecorderService) Start(ctx context.Context) error {
	recorder, err := NewRecorder(ctx, s.config, s.logger)
	if err != nil {
		return err
	}
	s.recorder = recorder

	if err := recorder.Listen(ctx, s.nc); err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}

	// Start background retention worker
	go recorder.StartRetentionWorker(ctx)

	return nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultRecorderConfig()
	if dbURL := os.Getenv("DB_URL"); dbURL != "" {
		config.DBURL = dbURL
	}
	if natsURL := os.Getenv("NATS_URL"); natsURL != "" {
		config.NATSURL = natsURL
	}
	if addr := os.Getenv("METRICS_ADDR"); addr != "" {
		config.MetricsAddr = addr
	}

	if err := config.Validate(); err != nil {
		logger.Error("Invalid configuration", "error", err)
		os.Exit(1)
	}

	common.StartMetricsServer(config.MetricsAddr)

	service, err := NewRecorderService(config, logger)
	if err != nil {
		logger.Error("Failed to create recorder service", "error", err)
		os.Exit(1)
	}
	defer service.Close()

	if err := service.Start(ctx); err != nil {
		logger.Error("Failed to start recorder service", "error", err)
		os.Exit(1)
	}

	<-ctx.Done()
	logger.Info("Shutting down recorder service...")
}
