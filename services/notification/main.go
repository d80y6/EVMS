package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
)

// NotificationConfig holds configuration for the notification service
type NotificationConfig struct {
	NATSURL string
}

// DefaultNotificationConfig returns default configuration values
func DefaultNotificationConfig() *NotificationConfig {
	return &NotificationConfig{
		NATSURL: getEnv("NATS_URL", "nats://nats:4222"),
	}
}

// NotificationType represents the type of notification
type NotificationType string

const (
	NotificationTypeEmail  NotificationType = "email"
	NotificationTypeWebhook NotificationType = "webhook"
	NotificationTypePush   NotificationType = "push"
)

// Notification represents a notification message
type Notification struct {
	Title   string           `json:"title"`
	Message string           `json:"message"`
	Type    NotificationType `json:"type"` // 'email', 'webhook', 'push'
}

// NotificationService handles notification delivery
type NotificationService struct {
	config *NotificationConfig
	nc     *nats.Conn
	logger *slog.Logger
	pushSub *nats.Subscription
}

// NewNotificationService creates a new notification service instance
func NewNotificationService(config *NotificationConfig, logger *slog.Logger) (*NotificationService, error) {
	nc, err := nats.Connect(config.NATSURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	return &NotificationService{
		config: config,
		nc:     nc,
		logger: logger,
	}, nil
}

// Start begins listening for notification requests
func (s *NotificationService) Start() error {
	var err error
	s.pushSub, err = s.nc.Subscribe("notifications.push", s.handlePushNotification)
	if err != nil {
		return fmt.Errorf("failed to subscribe to notifications.push: %w", err)
	}

	s.logger.Info("Notification Service started")
	return nil
}

// handlePushNotification processes incoming push notification requests
func (s *NotificationService) handlePushNotification(msg *nats.Msg) {
	var n Notification
	if err := json.Unmarshal(msg.Data, &n); err != nil {
		s.logger.Error("Failed to unmarshal notification", "error", err)
		return
	}

	if err := s.sendNotification(n); err != nil {
		s.logger.Error("Failed to send notification", "error", err, "title", n.Title)
	}
}

// sendNotification delivers a notification (placeholder for real integrations)
func (s *NotificationService) sendNotification(n Notification) error {
	s.logger.Info("Sending Notification", "title", n.Title, "message", n.Message, "type", n.Type)
	
	// In a real system, integrate with SendGrid (email), Twilio (SMS), or FCM (push)
	// Example integration points:
	// - Email: Use SendGrid API to send email notifications
	// - SMS: Use Twilio API to send text messages
	// - Push: Use Firebase Cloud Messaging for mobile push notifications
	
	return nil
}

// Close gracefully shuts down the service
func (s *NotificationService) Close() error {
	var errs []error

	if s.pushSub != nil {
		if err := s.pushSub.Unsubscribe(); err != nil {
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

	config := DefaultNotificationConfig()

	service, err := NewNotificationService(config, logger)
	if err != nil {
		logger.Error("Failed to initialize notification service", "error", err)
		os.Exit(1)
	}
	defer service.Close()

	if err := service.Start(); err != nil {
		logger.Error("Failed to start notification service", "error", err)
		os.Exit(1)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down Notification Service...")
}
