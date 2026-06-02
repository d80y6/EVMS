package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"github.com/dam-vms/dam/pkg/common"
)

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
	stopCh       chan struct{}
}

type OnvifEventsService struct {
	config        *OnvifEventsConfig
	nc            *nats.Conn
	logger        *slog.Logger
	mu            sync.Mutex
	subs          map[string]*subscription
	httpSrv       *http.Server
	healthHandler *common.HealthHandler
}

func NewOnvifEventsService(config *OnvifEventsConfig, logger *slog.Logger) (*OnvifEventsService, error) {
	nc, err := nats.Connect(config.NATSURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	svc := &OnvifEventsService{
		config:        config,
		nc:            nc,
		logger:        logger,
		subs:          make(map[string]*subscription),
		healthHandler: common.NewHealthHandler(),
	}
	svc.healthHandler.AddNATSChecker(nc, "nats")
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

	pullPointURL, err := s.createPullPointSubscription(req.DeviceURL, req.Username, req.Password)
	if err != nil {
		s.logger.Error("failed to create PullPoint subscription", "camera_id", req.CameraID, "error", err)
		http.Error(w, fmt.Sprintf("failed to create ONVIF subscription: %v", err), http.StatusBadGateway)
		return
	}

	sub := &subscription{
		CameraID:     req.CameraID,
		DeviceURL:    req.DeviceURL,
		PullPointURL: pullPointURL,
		Username:     req.Username,
		Password:     req.Password,
		stopCh:       make(chan struct{}),
	}

	s.mu.Lock()
	s.subs[req.CameraID] = sub
	s.mu.Unlock()

	go s.runSubscription(sub)

	s.logger.Info("ONVIF subscription created", "camera_id", req.CameraID, "pull_point_url", pullPointURL)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(subscribeResponse{
		CameraID:     req.CameraID,
		PullPointURL: pullPointURL,
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



func (s *OnvifEventsService) createPullPointSubscription(deviceURL, username, password string) (string, error) {
	subID := uuid.New().String()

	soapReq := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:wse="http://docs.oasis-open.org/wsn/b-2" xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
  <soap:Header>
    <wse:Identifier>uuid:%s</wse:Identifier>
  </soap:Header>
  <soap:Body>
    <wsnt:CreatePullPointSubscription>
      <wsnt:InitialTerminationTime>PT3600S</wsnt:InitialTerminationTime>
    </wsnt:CreatePullPointSubscription>
  </soap:Body>
</soap:Envelope>`, subID)

	eventServiceURL := fmt.Sprintf("%s/onvif/event_service", strings.TrimRight(deviceURL, "/"))

	req, err := http.NewRequest(http.MethodPost, eventServiceURL, strings.NewReader(soapReq))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")

	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send CreatePullPointSubscription: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("ONVIF device returned status %d: %s", resp.StatusCode, string(body))
	}

	return extractPullPointAddress(body)
}

func extractPullPointAddress(data []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var address string
	inAddress := false

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == "Address" {
				inAddress = true
			}
		case xml.CharData:
			if inAddress {
				address = string(t)
				return address, nil
			}
		}
	}

	return "", fmt.Errorf("no Address element found in response")
}

func (s *OnvifEventsService) pullMessages(pullPointURL string) ([]byte, error) {
	soapReq := `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
  <soap:Body>
    <wsnt:PullMessages>
      <wsnt:MaxNumberOfMessages>10</wsnt:MaxNumberOfMessages>
      <wsnt:Timeout>PT5S</wsnt:Timeout>
    </wsnt:PullMessages>
  </soap:Body>
</soap:Envelope>`

	req, err := http.NewRequest(http.MethodPost, pullPointURL, strings.NewReader(soapReq))
	if err != nil {
		return nil, fmt.Errorf("failed to create PullMessages request: %w", err)
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send PullMessages: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read PullMessages response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("PullMessages returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

const (
	EventTypeMotion = "motion"
	EventTypeTamper = "tamper"
	EventTypeAlarm  = "alarm"
)

var topicPatterns = map[string]string{
	"MotionAlarm":               EventTypeMotion,
	"CellMotionDetector/Motion": EventTypeMotion,
	"Motion":                    EventTypeMotion,
	"Tampering":                 EventTypeTamper,
	"Tamper":                    EventTypeTamper,
	"alarm":                     EventTypeAlarm,
	"Alarm":                     EventTypeAlarm,
}

func classifyTopic(topic string) string {
	topic = strings.TrimSpace(topic)
	for pattern, eventType := range topicPatterns {
		if strings.Contains(topic, pattern) {
			return eventType
		}
	}
	return ""
}

func extractTopics(data []byte) []string {
	var topics []string
	decoder := xml.NewDecoder(bytes.NewReader(data))
	inTopic := false
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == "Topic" {
				inTopic = true
			}
		case xml.CharData:
			if inTopic {
				topics = append(topics, string(t))
				inTopic = false
			}
		case xml.EndElement:
			if t.Name.Local == "Topic" {
				inTopic = false
			}
		}
	}
	return topics
}

func (s *OnvifEventsService) runSubscription(sub *subscription) {
	s.logger.Info("starting subscription poller", "camera_id", sub.CameraID)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			respBody, err := s.pullMessages(sub.PullPointURL)
			if err != nil {
				s.logger.Error("PullMessages failed", "camera_id", sub.CameraID, "error", err)
				continue
			}

			topics := extractTopics(respBody)
			for _, topic := range topics {
				eventType := classifyTopic(topic)
				if eventType == "" {
					continue
				}

				event := map[string]string{
					"camera_id":  sub.CameraID,
					"event_type": eventType,
					"timestamp":  time.Now().UTC().Format(time.RFC3339),
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
					s.logger.Info("published ONVIF event", "camera_id", sub.CameraID, "event_type", eventType, "topic", topic)
				}
			}

		case <-sub.stopCh:
			s.logger.Info("stopping subscription poller", "camera_id", sub.CameraID)
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
