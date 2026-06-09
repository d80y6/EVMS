package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/dam-vms/dam/pkg/onvif"
)

type OnvifEventRow struct {
	ID             string    `db:"id" json:"id"`
	CameraID       string    `db:"camera_id" json:"camera_id"`
	SubscriptionID string    `db:"subscription_id" json:"subscription_id,omitempty"`
	Topic          string    `db:"topic" json:"topic,omitempty"`
	Source         string    `db:"source" json:"source,omitempty"`
	EventType      string    `db:"event_type" json:"event_type"`
	Severity       string    `db:"severity" json:"severity,omitempty"`
	Message        string    `db:"message" json:"message,omitempty"`
	RawXML         string    `db:"raw_xml" json:"raw_xml,omitempty"`
	EventTime      time.Time `db:"event_time" json:"event_time"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
}

type OnvifEventsConfig struct {
	Port        string
	MetricsAddr string
	NATSURL     string
	DBURL       string
}

func DefaultOnvifEventsConfig() *OnvifEventsConfig {
	return &OnvifEventsConfig{
		Port:        common.GetEnv("ONVIF_EVENTS_PORT", ":8092"),
		MetricsAddr: common.GetEnv("METRICS_ADDR", ":2112"),
		NATSURL:     common.GetEnv("NATS_URL", "nats://nats:4222"),
		DBURL:       common.GetEnv("DB_URL", ""),
	}
}

type subscription struct {
	CameraID     string
	DeviceURL    string
	PullPointURL string
	Username     string
	Password     string
	client       *onvif.SOAPClient
	stopCh       chan struct{}
}

type OnvifEventsService struct {
	config        *OnvifEventsConfig
	nc            *nats.Conn
	db            *sqlx.DB
	logger        *slog.Logger
	mu            sync.Mutex
	subs          map[string]*subscription
	httpSrv       *http.Server
	healthHandler *common.HealthHandler
}

func NewOnvifEventsService(config *OnvifEventsConfig, logger *slog.Logger) (*OnvifEventsService, error) {
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
		cb := common.NewDBCircuitBreaker("onvif-events")
		db, err = common.ConnectDBWithCircuitBreaker(context.Background(), "postgres", config.DBURL, cb)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to database: %w", err)
		}
	}

	svc := &OnvifEventsService{
		config:        config,
		nc:            nc,
		db:            db,
		logger:        logger,
		subs:          make(map[string]*subscription),
		healthHandler: common.NewHealthHandler(),
	}
	svc.healthHandler.AddNATSChecker(nc, "nats")
	if db != nil {
		svc.healthHandler.AddDBChecker(db.DB, "postgres")
	}
	return svc, nil
}

func (s *OnvifEventsService) Start() error {
	mux := http.NewServeMux()
	mux.Handle("/onvif-events/subscribe", common.JWTAuthMiddleware(s.handleSubscribe))
	mux.Handle("/onvif-events/subscribe/", common.JWTAuthMiddleware(s.handleUnsubscribe))
	mux.Handle("/onvif-events/subscriptions", common.JWTAuthMiddleware(s.handleListSubscriptions))
	mux.HandleFunc("/health", s.healthHandler.Liveness)
	mux.HandleFunc("/ready", s.healthHandler.Readiness)

	s.httpSrv = &http.Server{
		Addr:    s.config.Port,
		Handler: common.RecoveryMiddleware(mux),
	}

	go func() {
		s.logger.Info("ONVIF Events Service listening", "port", s.config.Port)
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server error", "error", err)
		}
	}()

	return nil
}

func (s *OnvifEventsService) insertEvent(ctx context.Context, sub *subscription, evt onvif.ONVIFEvent, eventType string) error {
	if s.db == nil {
		return nil
	}

	now := evt.Timestamp
	if now.IsZero() {
		now = time.Now().UTC()
	}

	msg := ""
	if m, ok := evt.Data["message"].(string); ok {
		msg = m
	}
	severity := ""
	if sev, ok := evt.Data["severity"].(string); ok {
		severity = sev
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO onvif_events (camera_id, subscription_id, topic, source, event_type, severity, message, event_time, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())`,
		sub.CameraID, sub.PullPointURL, evt.Topic, "ONVIF", eventType, severity, msg, now)
	if err != nil {
		return fmt.Errorf("insert onvif event: %w", err)
	}
	return nil
}

func (s *OnvifEventsService) Close() error {
	s.mu.Lock()
	for _, sub := range s.subs {
		close(sub.stopCh)
	}
	s.subs = nil
	s.mu.Unlock()

	var errs []error

	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpSrv.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("HTTP server shutdown: %w", err))
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

type subscribeRequest struct {
	CameraID  string `json:"camera_id"`
	DeviceURL string `json:"onvif_device_url"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
}

type subscribeResponse struct {
	CameraID     string `json:"camera_id"`
	PullPointURL string `json:"pull_point_url"`
}

type subscriptionInfo struct {
	CameraID     string `json:"camera_id"`
	DeviceURL    string `json:"device_url"`
	PullPointURL string `json:"pull_point_url"`
}

func (s *OnvifEventsService) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req subscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.CameraID == "" || req.DeviceURL == "" {
		http.Error(w, "camera_id and onvif_device_url are required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	if _, exists := s.subs[req.CameraID]; exists {
		s.mu.Unlock()
		http.Error(w, "subscription already exists for this camera", http.StatusConflict)
		return
	}
	s.mu.Unlock()

	pullPointSub, client, err := s.createPullPointSubscription(req.DeviceURL, req.Username, req.Password)
	if err != nil {
		s.logger.Error("failed to create PullPoint subscription", "camera_id", req.CameraID, "error", err)
		http.Error(w, fmt.Sprintf("failed to create ONVIF subscription: %v", err), http.StatusBadGateway)
		return
	}

	sub := &subscription{
		CameraID:     req.CameraID,
		DeviceURL:    req.DeviceURL,
		PullPointURL: pullPointSub.Address,
		Username:     req.Username,
		Password:     req.Password,
		client:       client,
		stopCh:       make(chan struct{}),
	}

	s.mu.Lock()
	s.subs[req.CameraID] = sub
	s.mu.Unlock()

	go s.runSubscription(sub)

	s.logger.Info("ONVIF subscription created", "camera_id", req.CameraID, "pull_point_url", pullPointSub.Address)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(subscribeResponse{
		CameraID:     req.CameraID,
		PullPointURL: pullPointSub.Address,
	})
}

func (s *OnvifEventsService) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cameraID := strings.TrimPrefix(r.URL.Path, "/onvif-events/subscribe/")
	if cameraID == "" {
		http.Error(w, "camera_id required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	sub, exists := s.subs[cameraID]
	if !exists {
		s.mu.Unlock()
		http.Error(w, "subscription not found", http.StatusNotFound)
		return
	}
	delete(s.subs, cameraID)
	s.mu.Unlock()

	close(sub.stopCh)
	s.logger.Info("ONVIF subscription removed", "camera_id", cameraID)

	w.WriteHeader(http.StatusNoContent)
}

func (s *OnvifEventsService) handleListSubscriptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	subs := make([]subscriptionInfo, 0, len(s.subs))
	for _, sub := range s.subs {
		subs = append(subs, subscriptionInfo{
			CameraID:     sub.CameraID,
			DeviceURL:    sub.DeviceURL,
			PullPointURL: sub.PullPointURL,
		})
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(subs)
}



func (s *OnvifEventsService) createPullPointSubscription(deviceURL, username, password string) (*onvif.PullPointSubscription, *onvif.SOAPClient, error) {
	creds := &onvif.Credentials{Username: username, Password: password}
	client := onvif.NewSOAPClient(10*time.Second, creds)

	eventURL := onvif.BuildEventURL(deviceURL)
	sub, err := onvif.CreatePullPointSubscription(context.Background(), client, eventURL, 3600*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("CreatePullPointSubscription failed: %w", err)
	}

	return sub, client, nil
}

func (s *OnvifEventsService) runSubscription(sub *subscription) {
	s.logger.Info("starting subscription poller", "camera_id", sub.CameraID)

	pullTicker := time.NewTicker(5 * time.Second)
	renewTicker := time.NewTicker(30 * time.Minute)
	defer pullTicker.Stop()
	defer renewTicker.Stop()

	for {
		select {
		case <-pullTicker.C:
			events, err := onvif.PullMessages(context.Background(), sub.client, sub.PullPointURL, 10, 5*time.Second)
			if err != nil {
				s.logger.Error("PullMessages failed", "camera_id", sub.CameraID, "error", err)
				continue
			}

			for _, evt := range events {
				eventType := onvif.ClassifyEventTopic(evt.Topic)
				if eventType == "" {
					continue
				}

				if err := s.insertEvent(context.Background(), sub, evt, eventType); err != nil {
					s.logger.Error("failed to persist event", "camera_id", sub.CameraID, "error", err)
				}

				event := map[string]string{
					"camera_id":  sub.CameraID,
					"event_type": eventType,
					"timestamp":  evt.Timestamp.Format(time.RFC3339),
					"source":     "ONVIF",
				}

				data, err := json.Marshal(event)
				if err != nil {
					s.logger.Error("failed to marshal event", "error", err)
					continue
				}

				subject := fmt.Sprintf("camera.%s.onvif_events", sub.CameraID)
				if err := s.nc.Publish(subject, data); err != nil {
					s.logger.Error("failed to publish event", "subject", subject, "error", err)
				} else {
					s.logger.Info("published ONVIF event", "camera_id", sub.CameraID, "event_type", eventType, "topic", evt.Topic)
				}
			}

		case <-renewTicker.C:
			s.logger.Debug("renewing subscription", "camera_id", sub.CameraID)
			if err := onvif.RenewPullPointSubscription(context.Background(), sub.client, sub.PullPointURL, 3600*time.Second); err != nil {
				s.logger.Error("subscription renewal failed", "camera_id", sub.CameraID, "error", err)
			}

		case <-sub.stopCh:
			s.logger.Info("stopping subscription poller", "camera_id", sub.CameraID)
			if err := onvif.UnsubscribePullPoint(context.Background(), sub.client, sub.PullPointURL); err != nil {
				s.logger.Warn("Unsubscribe failed", "camera_id", sub.CameraID, "error", err)
			}
			return
		}
	}
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := common.InitTelemetry("onvif-events"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultOnvifEventsConfig()

	common.StartMetricsServer(config.MetricsAddr)
	common.StartResourceMonitor(ctx)

	service, err := NewOnvifEventsService(config, logger)
	if err != nil {
		logger.Error("Failed to initialize ONVIF Events service", "error", err)
		os.Exit(1)
	}
	defer service.Close()

	if err := service.Start(); err != nil {
		logger.Error("Failed to start ONVIF Events service", "error", err)
		os.Exit(1)
	}

	<-ctx.Done()
	logger.Info("Shutting down ONVIF Events Service...")
}
