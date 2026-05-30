package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
)

// MetadataConfig holds configuration for the metadata service
type MetadataConfig struct {
	DBURL      string
	NATSURL    string
	MaxRetries int
}

// DefaultMetadataConfig returns default configuration values
func DefaultMetadataConfig() *MetadataConfig {
	return &MetadataConfig{
		DBURL:      getEnv("DB_URL", ""),
		NATSURL:    getEnv("NATS_URL", "nats://nats:4222"),
		MaxRetries: 3,
	}
}

// Detection represents an AI detection event
type Detection struct {
	Label      string    `json:"label"`
	Confidence float64   `json:"confidence"`
	BBox       []float64 `json:"bbox"`
	Embedding  []float32 `json:"embedding,omitempty"`
}

// MetadataService handles AI event metadata storage
type MetadataService struct {
	config *MetadataConfig
	db     *sqlx.DB
	nc     *nats.Conn
	logger *slog.Logger
	sub    *nats.Subscription
}

// NewMetadataService creates a new metadata service instance
func NewMetadataService(config *MetadataConfig, logger *slog.Logger) (*MetadataService, error) {
	if config.DBURL == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	db, err := sqlx.Connect("postgres", config.DBURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	nc, err := nats.Connect(config.NATSURL)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	return &MetadataService{
		config: config,
		db:     db,
		nc:     nc,
		logger: logger,
	}, nil
}

// Start begins listening for AI events
func (s *MetadataService) Start() error {
	var err error
	s.sub, err = s.nc.Subscribe("camera.*.events", s.handleAIEvent)
	if err != nil {
		return fmt.Errorf("failed to subscribe to NATS subject: %w", err)
	}

	s.logger.Info("AI Metadata Service (Vector-Enabled) started")
	return nil
}

// handleAIEvent processes incoming AI detection events
func (s *MetadataService) handleAIEvent(msg *nats.Msg) {
	var detections []Detection
	if err := json.Unmarshal(msg.Data, &detections); err != nil {
		s.logger.Error("Failed to unmarshal AI event", "error", err)
		return
	}

	// Subject format: camera.<id>.events
	parts := strings.Split(msg.Subject, ".")
	if len(parts) < 2 {
		s.logger.Warn("Invalid NATS subject for AI event", "subject", msg.Subject)
		return
	}
	cameraID := parts[1]

	for _, d := range detections {
		if err := s.storeDetection(cameraID, d); err != nil {
			s.logger.Error("Failed to store AI event", "error", err, "camera_id", cameraID, "label", d.Label)
		}
	}
}

// storeDetection persists a single detection to the database
func (s *MetadataService) storeDetection(cameraID string, detection Detection) error {
	bbox, err := json.Marshal(detection.BBox)
	if err != nil {
		return fmt.Errorf("failed to marshal bounding box: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if len(detection.Embedding) > 0 {
		embeddingJSON, err := json.Marshal(detection.Embedding)
		if err != nil {
			return fmt.Errorf("failed to marshal embedding: %w", err)
		}
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO ai_events (camera_id, object_type, confidence, bounding_box, embedding, event_time)
                          VALUES ($1, $2, $3, $4, $5, NOW())`,
			cameraID, detection.Label, detection.Confidence, string(bbox), string(embeddingJSON))
		return err
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO ai_events (camera_id, object_type, confidence, bounding_box, event_time)
                  VALUES ($1, $2, $3, $4, NOW())`,
		cameraID, detection.Label, detection.Confidence, string(bbox))
	return err
}

// Close gracefully shuts down the service
func (s *MetadataService) Close() error {
	var errs []error

	if s.sub != nil {
		if err := s.sub.Unsubscribe(); err != nil {
			errs = append(errs, fmt.Errorf("failed to unsubscribe: %w", err))
		}
	}

	if s.nc != nil {
		s.nc.Close()
	}

	if err := s.db.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close database: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errs)
	}
	return nil
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

	config := DefaultMetadataConfig()

	service, err := NewMetadataService(config, logger)
	if err != nil {
		logger.Error("Failed to initialize metadata service", "error", err)
		os.Exit(1)
	}
	defer service.Close()

	if err := service.Start(); err != nil {
		logger.Error("Failed to start metadata service", "error", err)
		os.Exit(1)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down AI Metadata Service...")
}
