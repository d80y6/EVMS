package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/nats-io/nats.go"
)

// NotificationConfig holds configuration for the notification service
type NotificationConfig struct {
	NATSURL string
}

// DefaultNotificationConfig returns default configuration values
func DefaultNotificationConfig() *NotificationConfig {
	return &NotificationConfig{
		NATSURL: common.GetEnv("NATS_URL", "nats://nats:4222"),
	}
}

// NotificationType represents the type of notification
type NotificationType string

const (
	NotificationTypeEmail   NotificationType = "email"
	NotificationTypeWebhook NotificationType = "webhook"
	NotificationTypePush    NotificationType = "push"
)

// Notification represents a notification message
type Notification struct {
	Title   string           `json:"title"`
	Message string           `json:"message"`
	Type    NotificationType `json:"type"` // 'email', 'webhook', 'push'
}

// NotificationService handles notification delivery
type NotificationService struct {
	config    *NotificationConfig
	nc        *nats.Conn
	logger    *slog.Logger
	pushSub   *nats.Subscription
	healthSrv *http.Server
}

// NewNotificationService creates a new notification service instance
func NewNotificationService(config *NotificationConfig, logger *slog.Logger) (*NotificationService, error) {
	nc, err := nats.Connect(config.NATSURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	h := common.NewHealthHandler()
	if nc != nil {
		h.AddNATSChecker(nc, "nats")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.Liveness)
	mux.HandleFunc("/ready", h.Readiness)

	return &NotificationService{
		config:    config,
		nc:        nc,
		logger:    logger,
		healthSrv: &http.Server{Addr: ":8090", Handler: mux},
	}, nil
}

// Start begins listening for notification requests
func (s *NotificationService) Start() error {
	var err error
	s.pushSub, err = s.nc.QueueSubscribe("notifications.push", "notification", s.handlePushNotification)
	if err != nil {
		return fmt.Errorf("failed to subscribe to notifications.push: %w", err)
	}
	s.pushSub.SetPendingLimits(1024, 64*1024*1024)

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

// sendNotification delivers a notification based on type
func (s *NotificationService) sendNotification(n Notification) error {
	s.logger.Info("Sending Notification", "title", n.Title, "message", n.Message, "type", n.Type)

	switch n.Type {
	case NotificationTypeWebhook:
		if webhookURL := os.Getenv("WEBHOOK_URL"); webhookURL != "" {
			return s.sendWebhook(n, webhookURL)
		}
	case NotificationTypePush:
		if fcmKey := os.Getenv("FCM_SERVER_KEY"); fcmKey != "" {
			return s.sendFCM(n, fcmKey)
		}
	case NotificationTypeEmail:
		if smtpServer := os.Getenv("SMTP_SERVER"); smtpServer != "" {
			return s.sendEmail(n)
		}
	}

	s.logger.Warn("No notification provider configured, notification will be logged only",
		"title", n.Title, "type", n.Type)
	return nil
}

// sendWebhook delivers a notification via HTTP webhook
func (s *NotificationService) sendWebhook(n Notification, webhookURL string) error {
	if err := common.ValidateWebhookURL(webhookURL); err != nil {
		return fmt.Errorf("webhook URL validation failed: %w", err)
	}

	body, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned non-success status: %d", resp.StatusCode)
	}

	s.logger.Info("Webhook notification sent", "title", n.Title, "url", webhookURL)
	return nil
}

// sendFCM delivers a notification via Firebase Cloud Messaging (placeholder)
func (s *NotificationService) sendFCM(n Notification, _ string) error {
	return fmt.Errorf("FCM integration not yet implemented")
}

// sendEmail delivers a notification via SMTP (placeholder)
func (s *NotificationService) sendEmail(n Notification) error {
	return fmt.Errorf("SMTP integration not yet implemented")
}

// Close gracefully shuts down the service
func (s *NotificationService) Close() error {
	var errs []error

	if s.healthSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.healthSrv.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown health server: %w", err))
		}
	}

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

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := common.InitTelemetry("notification"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultNotificationConfig()

	common.StartMetricsServer(common.GetEnv("METRICS_ADDR", ":2112"))
	common.StartResourceMonitor(ctx)

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

	go func() {
		logger.Info("Starting health HTTP server", "addr", ":8090")
		if err := service.healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Health server error", "error", err)
		}
	}()

	<-ctx.Done()
	logger.Info("Shutting down Notification Service...")
}
