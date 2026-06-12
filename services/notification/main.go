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
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"

	"github.com/dam-vms/dam/pkg/common"
)

type Webhook struct {
	ID         string    `json:"id" db:"id"`
	Name       string    `json:"name" db:"name"`
	URL        string    `json:"url" db:"url"`
	EventTypes []string  `json:"event_types" db:"event_types"`
	CameraIDs  []string  `json:"camera_ids" db:"camera_ids"`
	Enabled    bool      `json:"enabled" db:"enabled"`
	Secret     string    `json:"secret,omitempty" db:"secret"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}

// NotificationConfig holds configuration for the notification service
type NotificationConfig struct {
	NATSURL string
	DBURL   string
}

// DefaultNotificationConfig returns default configuration values
func DefaultNotificationConfig() *NotificationConfig {
	return &NotificationConfig{
		NATSURL: common.GetEnv("NATS_URL", "nats://nats:4222"),
		DBURL:   os.Getenv("DB_URL"),
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
	config          *NotificationConfig
	nc              *nats.Conn
	db              *sqlx.DB
	logger          *slog.Logger
	pushSub         *nats.Subscription
	httpSrv         *http.Server
	healthSrv       *http.Server
	channelMgr      *ChannelManager
	rateLimiter     *ChannelRateLimiter
	configMgr       *ConfigManager
	notificationSub *nats.Subscription
}

// NewNotificationService creates a new notification service instance
func NewNotificationService(config *NotificationConfig, logger *slog.Logger) (*NotificationService, error) {
	nc, err := nats.Connect(config.NATSURL, append(common.NATSTLSOptions(),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	var db *sqlx.DB
	if config.DBURL != "" {
		cb := common.NewDBCircuitBreaker("notification")
		db, err = common.ConnectDBWithCircuitBreaker(context.Background(), "postgres", config.DBURL, cb)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to database: %w", err)
		}
	}

	h := common.NewHealthHandler()
	if nc != nil {
		h.AddNATSChecker(nc, "nats")
	}
	if db != nil {
		h.AddDBChecker(db.DB, "postgres")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.Liveness)
	mux.HandleFunc("/ready", h.Readiness)
	mux.HandleFunc("/api/webhooks", func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			listWebhooks(db, w, r)
		case http.MethodPost:
			createWebhook(db, w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/webhooks/", func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/webhooks/")
		if id == "" {
			http.Error(w, "webhook id required", http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodPut:
			updateWebhook(db, w, r, id)
		case http.MethodDelete:
			deleteWebhook(db, w, r, id)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Notification channels
	mux.HandleFunc("/api/channels", handleChannels(db, logger))
	mux.HandleFunc("/api/channels/", handleChannelByID(db, logger))

	// Notification templates
	mux.HandleFunc("/api/templates", handleTemplates(db, logger))
	mux.HandleFunc("/api/templates/", handleTemplateByID(db, logger))

	// Notification log
	mux.HandleFunc("/api/notification-log", handleNotificationLog(db, logger))

	// System config
	cm := NewConfigManager(db)
	if err := cm.SeedDefaults(); err != nil {
		logger.Error("failed to seed default config", "error", err)
	}
	mux.HandleFunc("/api/admin/config", handleAdminConfig(db, cm))
	mux.HandleFunc("/api/admin/config/", handleAdminConfigCategory(db))
	mux.Handle("/api/admin/config/history", common.JWTAuthMiddleware(handleConfigHistory(db)))
	mux.Handle("/api/admin/config/export", common.JWTAuthMiddleware(handleConfigExport(db)))
	mux.Handle("/api/admin/config/import", common.JWTAuthMiddleware(handleConfigImport(db)))

	chMgr := NewChannelManager(db, logger)
	chMgr.LoadFromDB(context.Background())

	rl := NewChannelRateLimiter()

	return &NotificationService{
		config:      config,
		nc:          nc,
		db:          db,
		logger:      logger,
		healthSrv:   &http.Server{Addr: ":8090", Handler: mux},
		channelMgr:  chMgr,
		rateLimiter: rl,
		configMgr:   cm,
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

	s.notificationSub, err = s.nc.QueueSubscribe("alerts.triggered", "notification", s.handleAlertNotification)
	if err != nil {
		return fmt.Errorf("failed to subscribe to alerts.triggered: %w", err)
	}
	s.notificationSub.SetPendingLimits(1024, 64*1024*1024)

	s.logger.Info("Notification Service started")
	return nil
}

// handleAlertNotification processes triggered alerts and sends via channels
func (s *NotificationService) handleAlertNotification(msg *nats.Msg) {
	var alert map[string]interface{}
	if err := json.Unmarshal(msg.Data, &alert); err != nil {
		s.logger.Error("Failed to unmarshal alert", "error", err)
		return
	}

	cameraID, _ := alert["camera_id"].(string)
	ruleName, _ := alert["rule_name"].(string)
	timestamp, _ := alert["timestamp"].(string)
	confidence, _ := alert["confidence"].(float64)

	title := "Security Alert"
	message := fmt.Sprintf("Rule '%s' triggered on camera %s", ruleName, cameraID)

	emailBody, err := renderAlertEmail(AlertTemplateData{
		Title:      title,
		CameraID:   cameraID,
		Timestamp:  timestamp,
		Message:    message,
		Confidence: fmt.Sprintf("%.0f%%", confidence*100),
	})
	if err != nil {
		s.logger.Error("Failed to render alert email", "error", err)
	}

	n := Notification{
		Title:   title,
		Message: message,
		Type:    NotificationTypeEmail,
	}

	if s.channelMgr != nil {
		s.channelMgr.mu.RLock()
		for id, ch := range s.channelMgr.channels {
			if s.rateLimiter.Allow(id, 60) {
				body := n.Message
				if ch.Type() == ChannelEmail && emailBody != "" {
					body = emailBody
				}
				err := retryWithBackoff(3, func() error {
					return ch.Send("", n.Title, body)
				})
				if err != nil {
					s.logger.Error("channel delivery failed for alert", "channel_id", id, "error", err)
				}
			}
		}
		s.channelMgr.mu.RUnlock()
	}

	s.logger.Info("Alert notification processed", "camera", cameraID, "rule", ruleName)
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

	// Use configured channels first
	if s.channelMgr != nil {
		s.channelMgr.mu.RLock()
		var matchingChannels []string
		for id, ch := range s.channelMgr.channels {
			if string(ch.Type()) == string(n.Type) {
				matchingChannels = append(matchingChannels, id)
			}
		}
		s.channelMgr.mu.RUnlock()

		if len(matchingChannels) > 0 {
			for _, chID := range matchingChannels {
				ch, ok := s.channelMgr.GetChannel(chID)
				if !ok {
					continue
				}
				if !s.rateLimiter.Allow(chID, 60) {
					s.logger.Warn("rate limit exceeded for channel", "channel_id", chID)
					continue
				}
				err := retryWithBackoff(3, func() error {
					return ch.Send("", n.Title, n.Message)
				})
				if err != nil {
					s.logger.Error("channel delivery failed", "channel_id", chID, "error", err)
					if s.db != nil {
						s.db.Exec(`INSERT INTO notification_log (id, channel_id, recipient, subject, status, error, created_at) VALUES ($1, $2, $3, $4, 'failed', $5, NOW())`,
							uuid.New().String(), chID, "", n.Title, err.Error())
					}
				} else {
					s.logger.Info("notification sent via channel", "channel_id", chID)
					if s.db != nil {
						s.db.Exec(`INSERT INTO notification_log (id, channel_id, recipient, subject, status, sent_at, created_at) VALUES ($1, $2, $3, $4, 'sent', NOW(), NOW())`,
							uuid.New().String(), chID, "", n.Title)
					}
				}
			}
			return nil
		}
	}

	// Fall back to old methods
	switch n.Type {
	case NotificationTypeWebhook:
		return s.sendWebhooks(n)
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

// sendWebhooks delivers a notification via HTTP webhook to all matching webhooks
func (s *NotificationService) sendWebhooks(n Notification) error {
	if s.db == nil {
		webhookURL := os.Getenv("WEBHOOK_URL")
		if webhookURL != "" {
			return s.sendWebhook(n, webhookURL, "")
		}
		return nil
	}

	var webhooks []Webhook
	if err := s.db.Select(&webhooks, "SELECT id, name, url, event_types, camera_ids, enabled, secret, created_at, updated_at FROM webhooks WHERE enabled = true"); err != nil {
		s.logger.Error("failed to query webhooks", "error", err)
		return nil
	}

	for _, w := range webhooks {
		if !s.webhookMatches(w, n) {
			continue
		}
		if err := s.sendWebhook(n, w.URL, w.Secret); err != nil {
			s.logger.Error("webhook delivery failed", "webhook_id", w.ID, "url", w.URL, "error", err)
		}
	}
	return nil
}

func (s *NotificationService) webhookMatches(w Webhook, n Notification) bool {
	if !w.Enabled {
		return false
	}
	if len(w.EventTypes) > 0 {
		match := false
		for _, et := range w.EventTypes {
			if et == string(n.Type) || et == "*" {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	return true
}

// sendWebhook delivers a notification via HTTP webhook
func (s *NotificationService) sendWebhook(n Notification, webhookURL string, secret string) error {
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
	if secret != "" {
		req.Header.Set("X-Webhook-Secret", secret)
	}

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

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func listWebhooks(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	var webhooks []Webhook
	if err := db.Select(&webhooks, "SELECT id, name, url, event_types, camera_ids, enabled, secret, created_at, updated_at FROM webhooks ORDER BY created_at DESC"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list webhooks"})
		return
	}
	if webhooks == nil {
		webhooks = []Webhook{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"webhooks": webhooks})
}

func createWebhook(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string   `json:"name"`
		URL        string   `json:"url"`
		EventTypes []string `json:"event_types"`
		CameraIDs  []string `json:"camera_ids"`
		Secret     string   `json:"secret,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Name == "" || req.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and url are required"})
		return
	}

	id := uuid.New().String()
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO webhooks (id, name, url, event_types, camera_ids, enabled, secret, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, true, $6, $7, $7)`,
		id, req.Name, req.URL, pqStringArray(req.EventTypes), pqStringArray(req.CameraIDs), req.Secret, now,
	); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create webhook"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id, "status": "created"})
}

func updateWebhook(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Name       *string  `json:"name,omitempty"`
		URL        *string  `json:"url,omitempty"`
		EventTypes []string `json:"event_types,omitempty"`
		CameraIDs  []string `json:"camera_ids,omitempty"`
		Secret     *string  `json:"secret,omitempty"`
		Enabled    *bool    `json:"enabled,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.URL != nil {
		setClauses = append(setClauses, fmt.Sprintf("url = $%d", argIdx))
		args = append(args, *req.URL)
		argIdx++
	}
	if req.EventTypes != nil {
		setClauses = append(setClauses, fmt.Sprintf("event_types = $%d", argIdx))
		args = append(args, pqStringArray(req.EventTypes))
		argIdx++
	}
	if req.CameraIDs != nil {
		setClauses = append(setClauses, fmt.Sprintf("camera_ids = $%d", argIdx))
		args = append(args, pqStringArray(req.CameraIDs))
		argIdx++
	}
	if req.Secret != nil {
		setClauses = append(setClauses, fmt.Sprintf("secret = $%d", argIdx))
		args = append(args, *req.Secret)
		argIdx++
	}
	if req.Enabled != nil {
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", argIdx))
		args = append(args, *req.Enabled)
		argIdx++
	}

	if len(setClauses) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	args = append(args, id)

	query := fmt.Sprintf("UPDATE webhooks SET %s WHERE id = $%d", strings.Join(setClauses, ", "), argIdx)
	result, err := db.Exec(query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update webhook"})
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "webhook not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func deleteWebhook(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	result, err := db.Exec("DELETE FROM webhooks WHERE id = $1", id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete webhook"})
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "webhook not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func pqStringArray(s []string) interface{} {
	if s == nil {
		s = []string{}
	}
	return "{" + strings.Join(s, ",") + "}"
}

// sendFCM delivers a notification via Firebase Cloud Messaging
func (s *NotificationService) sendFCM(n Notification, serverKey string) error {
	ch := NewPushChannel(PushConfig{ServerKey: serverKey}, s.logger)
	return ch.Send("", n.Title, n.Message)
}

// sendEmail delivers a notification via SMTP
func (s *NotificationService) sendEmail(n Notification) error {
	host := os.Getenv("SMTP_SERVER")
	port := 587
	if p := os.Getenv("SMTP_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}
	username := os.Getenv("SMTP_USERNAME")
	password := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SMTP_FROM")
	if from == "" {
		from = "noreply@evms.local"
	}

	ch := NewEmailChannel(EmailConfig{
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		From:     from,
	}, s.logger)

	// Use configured notification log to get recipient
	recipient := os.Getenv("NOTIFICATION_EMAIL_TO")
	if recipient == "" {
		recipient = "admin@evms.local"
	}

	return ch.Send(recipient, n.Title, n.Message)
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

	if s.db != nil {
		if err := s.db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close database: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errs)
	}
	return nil
}

func main() {
	logger := common.NewLogger("notification")
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
