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

	"github.com/dam-vms/dam/pkg/common"
	"github.com/nats-io/nats.go"
)

// EventProcConfig holds configuration for the event processing service
type EventProcConfig struct {
	NATSURL                   string
	PersonConfidenceThreshold float64
}

// DefaultEventProcConfig returns default configuration values
func DefaultEventProcConfig() *EventProcConfig {
	return &EventProcConfig{
		NATSURL:                   common.GetEnv("NATS_URL", "nats://nats:4222"),
		PersonConfidenceThreshold: 0.8,
	}
}

// Detection represents an AI detection event
type Detection struct {
	Label      string    `json:"label"`
	Confidence float64   `json:"confidence"`
	BBox       []float64 `json:"bbox"`
}

// EventProcessor handles AI event correlation and notification triggering
type EventProcessor struct {
	config   *EventProcConfig
	nc       *nats.Conn
	logger   *slog.Logger
	eventSub *nats.Subscription
}

// NewEventProcessor creates a new event processor instance
func NewEventProcessor(config *EventProcConfig, logger *slog.Logger) (*EventProcessor, error) {
	nc, err := nats.Connect(config.NATSURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	return &EventProcessor{
		config: config,
		nc:     nc,
		logger: logger,
	}, nil
}

// Start begins listening for camera events
func (s *EventProcessor) Start() error {
	var err error
	s.eventSub, err = s.nc.QueueSubscribe("camera.*.events", "event-proc", s.handleCameraEvent, nats.PendingLimits(1024, 64*1024*1024))
	if err != nil {
		return fmt.Errorf("failed to subscribe to camera events: %w", err)
	}

	s.logger.Info("Event Processing Service started",
		"person_confidence_threshold", s.config.PersonConfidenceThreshold)
	return nil
}

// handleCameraEvent processes incoming camera detection events
func (s *EventProcessor) handleCameraEvent(msg *nats.Msg) {
	var detections []Detection
	if err := json.Unmarshal(msg.Data, &detections); err != nil {
		s.logger.Error("Failed to unmarshal detections", "error", err)
		return
	}

	for _, d := range detections {
		if s.shouldTriggerNotification(d) {
			s.triggerNotification(msg.Subject, d)
		}
	}
}

// shouldTriggerNotification determines if a detection should trigger a notification
func (s *EventProcessor) shouldTriggerNotification(d Detection) bool {
	return d.Label == "person" && d.Confidence > s.config.PersonConfidenceThreshold
}

// triggerNotification sends a notification for a significant detection
func (s *EventProcessor) triggerNotification(subject string, detection Detection) {
	cameraID := extractCameraID(subject)

	s.logger.Info("Person detected! Triggering notification.",
		"camera", subject,
		"camera_id", cameraID,
		"confidence", detection.Confidence)

	notification := map[string]string{
		"title":   "Security Alert",
		"message": fmt.Sprintf("Person detected on camera %s with %.0f%% confidence", cameraID, detection.Confidence*100),
	}

	data, err := json.Marshal(notification)
	if err != nil {
		s.logger.Error("Failed to marshal notification", "error", err)
		return
	}

	if err := s.nc.Publish("notifications.push", data); err != nil {
		s.logger.Error("Failed to publish notification", "error", err)
	}
}

// extractCameraID extracts the camera ID from a NATS subject
func extractCameraID(subject string) string {
	parts := strings.Split(subject, ".")
	if len(parts) >= 2 {
		return parts[1]
	}
	return "unknown"
}

// Close gracefully shuts down the service
func (s *EventProcessor) Close() error {
	var errs []error

	if s.eventSub != nil {
		if err := s.eventSub.Unsubscribe(); err != nil {
			errs = append(errs, fmt.Errorf("failed to unsubscribe: %w", err))
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

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultEventProcConfig()

	common.StartMetricsServer(common.GetEnv("METRICS_ADDR", ":2112"))

	service, err := NewEventProcessor(config, logger)
	if err != nil {
		logger.Error("Failed to initialize event processor", "error", err)
		os.Exit(1)
	}
	defer service.Close()

	if err := service.Start(); err != nil {
		logger.Error("Failed to start event processor", "error", err)
		os.Exit(1)
	}

	<-ctx.Done()
	logger.Info("Shutting down Event Processing Service...")
}
