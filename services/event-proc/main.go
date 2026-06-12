package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
)

type EventProcConfig struct {
	NATSURL                   string
	PersonConfidenceThreshold float64
	AlertAdminPort            string
	DBURL                     string
}

func DefaultEventProcConfig() *EventProcConfig {
	return &EventProcConfig{
		NATSURL:                   common.GetEnv("NATS_URL", "nats://nats:4222"),
		PersonConfidenceThreshold: 0.8,
		AlertAdminPort:            common.GetEnv("ALERT_ADMIN_PORT", ":8093"),
		DBURL:                     os.Getenv("DB_URL"),
	}
}

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

type Detection struct {
	Label      string    `json:"label"`
	Confidence float64   `json:"confidence"`
	BBox       []float64 `json:"bbox"`
}

type Track struct {
	TrackID    string    `json:"track_id"`
	Label      string    `json:"label"`
	Confidence float64   `json:"confidence"`
	BBox       []float64 `json:"bbox"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
	CameraID   string    `json:"camera_id"`
}

type AlertRule struct {
	ID            string  `json:"id"`
	CameraID      string  `json:"camera_id"`
	Name          string  `json:"name"`
	ObjectType    string  `json:"object_type"`
	Zone          string  `json:"zone"`
	MinConfidence float64 `json:"min_confidence"`
	Action        string  `json:"action"`
	Enabled       bool    `json:"enabled"`
	CreatedAt     string  `json:"created_at"`
}

type AlertRuleManager struct {
	mu    sync.RWMutex
	rules map[string]*AlertRule
	db    *sqlx.DB
}

func NewAlertRuleManager(db *sqlx.DB) *AlertRuleManager {
	m := &AlertRuleManager{
		rules: make(map[string]*AlertRule),
		db:    db,
	}
	m.loadFromDB()
	return m
}

func (m *AlertRuleManager) loadFromDB() {
	if m.db == nil {
		return
	}
	var rows []struct {
		ID            string    `db:"id"`
		CameraID      string    `db:"camera_id"`
		Name          string    `db:"name"`
		ObjectType    string    `db:"object_type"`
		Zone          string    `db:"zone"`
		MinConfidence float64   `db:"min_confidence"`
		Action        string    `db:"action"`
		Enabled       bool      `db:"enabled"`
		CreatedAt     time.Time `db:"created_at"`
	}
	if err := m.db.Select(&rows, "SELECT id, camera_id, name, object_type, zone, min_confidence, action, enabled, created_at FROM alert_rules"); err != nil {
		slog.Warn("Failed to load alert rules from DB", "error", err)
		return
	}
	for _, row := range rows {
		m.rules[row.ID] = &AlertRule{
			ID:            row.ID,
			CameraID:      row.CameraID,
			Name:          row.Name,
			ObjectType:    row.ObjectType,
			Zone:          row.Zone,
			MinConfidence: row.MinConfidence,
			Action:        row.Action,
			Enabled:       row.Enabled,
			CreatedAt:     row.CreatedAt.Format(time.RFC3339),
		}
	}
	slog.Info("Loaded alert rules from database", "count", len(rows))
}

func (m *AlertRuleManager) saveRule(rule *AlertRule) error {
	if m.db == nil {
		return nil
	}
	_, err := m.db.Exec(`INSERT INTO alert_rules (id, camera_id, name, object_type, zone, min_confidence, action, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (id) DO UPDATE SET camera_id=$2, name=$3, object_type=$4, zone=$5, min_confidence=$6, action=$7, enabled=$8, updated_at=NOW()`,
		rule.ID, rule.CameraID, rule.Name, rule.ObjectType, rule.Zone, rule.MinConfidence, rule.Action, rule.Enabled, rule.CreatedAt)
	return err
}

func (m *AlertRuleManager) deleteRuleFromDB(id string) error {
	if m.db == nil {
		return nil
	}
	_, err := m.db.Exec("DELETE FROM alert_rules WHERE id=$1", id)
	return err
}

func (m *AlertRuleManager) List(cameraID string) []*AlertRule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*AlertRule
	for _, r := range m.rules {
		if cameraID == "" || r.CameraID == cameraID {
			result = append(result, r)
		}
	}
	return result
}

func (m *AlertRuleManager) Create(rule *AlertRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rule.ID = uuid.New().String()
	rule.CreatedAt = time.Now().Format(time.RFC3339)
	if err := m.saveRule(rule); err != nil {
		return fmt.Errorf("failed to persist alert rule: %w", err)
	}
	m.rules[rule.ID] = rule
	return nil
}

func (m *AlertRuleManager) Update(rule *AlertRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.rules[rule.ID]
	if !ok {
		return fmt.Errorf("alert rule not found: %s", rule.ID)
	}

	rule.CreatedAt = existing.CreatedAt
	if err := m.saveRule(rule); err != nil {
		return fmt.Errorf("failed to persist alert rule update: %w", err)
	}
	m.rules[rule.ID] = rule
	return nil
}

func (m *AlertRuleManager) Delete(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.rules[id]
	if !ok {
		return false
	}
	if err := m.deleteRuleFromDB(id); err != nil {
		slog.Error("Failed to delete alert rule from DB", "id", id, "error", err)
		return false
	}
	delete(m.rules, id)
	return true
}

func (m *AlertRuleManager) GetMatching(cameraID, objectType string, confidence float64, center [2]float64) []*AlertRule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matches []*AlertRule
	for _, r := range m.rules {
		if !r.Enabled {
			continue
		}
		if r.CameraID != cameraID {
			continue
		}
		if r.ObjectType != "any" && r.ObjectType != objectType {
			continue
		}
		if confidence < r.MinConfidence {
			continue
		}
		if r.Zone != "" {
			var polygon [][2]float64
			if err := json.Unmarshal([]byte(r.Zone), &polygon); err != nil {
				continue
			}
			if !pointInPolygon(center, polygon) {
				continue
			}
		}
		matches = append(matches, r)
	}
	return matches
}

type Tracker struct {
	mu           sync.Mutex
	tracks       map[string][]*Track
	ioUThreshold float64
	trackTimeout time.Duration
}

func NewTracker(ioUThreshold float64, trackTimeout time.Duration) *Tracker {
	return &Tracker{
		tracks:       make(map[string][]*Track),
		ioUThreshold: ioUThreshold,
		trackTimeout: trackTimeout,
	}
}

func (t *Tracker) computeIoU(box1, box2 []float64) float64 {
	if len(box1) < 4 || len(box2) < 4 {
		return 0
	}

	x1 := math.Max(box1[0], box2[0])
	y1 := math.Max(box1[1], box2[1])
	x2 := math.Min(box1[2], box2[2])
	y2 := math.Min(box1[3], box2[3])

	if x2 <= x1 || y2 <= y1 {
		return 0
	}

	interArea := (x2 - x1) * (y2 - y1)
	box1Area := (box1[2] - box1[0]) * (box1[3] - box1[1])
	box2Area := (box2[2] - box2[0]) * (box2[3] - box2[1])

	unionArea := box1Area + box2Area - interArea
	if unionArea <= 0 {
		return 0
	}

	return interArea / unionArea
}

func (t *Tracker) matchDetections(cameraID string, detections []Detection) []Track {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()

	if t.tracks[cameraID] == nil {
		t.tracks[cameraID] = make([]*Track, 0)
	}

	var matchedTracks []Track
	usedTracks := make(map[int]bool)

	for _, d := range detections {
		bestIdx := -1
		bestIoU := t.ioUThreshold

		for i, tr := range t.tracks[cameraID] {
			if usedTracks[i] {
				continue
			}
			if tr.Label != d.Label {
				continue
			}
			iou := t.computeIoU(tr.BBox, d.BBox)
			if iou > bestIoU {
				bestIoU = iou
				bestIdx = i
			}
		}

		if bestIdx >= 0 {
			usedTracks[bestIdx] = true
			tr := t.tracks[cameraID][bestIdx]
			tr.BBox = d.BBox
			tr.Confidence = d.Confidence
			tr.LastSeen = now

			matchedTracks = append(matchedTracks, Track{
				TrackID:    tr.TrackID,
				Label:      tr.Label,
				Confidence: tr.Confidence,
				BBox:       tr.BBox,
				FirstSeen:  tr.FirstSeen,
				LastSeen:   tr.LastSeen,
				CameraID:   cameraID,
			})
		} else {
			track := &Track{
				TrackID:    uuid.New().String(),
				Label:      d.Label,
				Confidence: d.Confidence,
				BBox:       d.BBox,
				FirstSeen:  now,
				LastSeen:   now,
				CameraID:   cameraID,
			}
			t.tracks[cameraID] = append(t.tracks[cameraID], track)

			matchedTracks = append(matchedTracks, Track{
				TrackID:    track.TrackID,
				Label:      track.Label,
				Confidence: track.Confidence,
				BBox:       track.BBox,
				FirstSeen:  track.FirstSeen,
				LastSeen:   track.LastSeen,
				CameraID:   cameraID,
			})
		}
	}

	return matchedTracks
}

func (t *Tracker) cleanupStaleTracks() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()

	for cameraID, tracks := range t.tracks {
		active := tracks[:0]
		for _, tr := range tracks {
			if now.Sub(tr.LastSeen) < t.trackTimeout {
				active = append(active, tr)
			}
		}
		t.tracks[cameraID] = active
	}
}

func (t *Tracker) StartCleanupLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.cleanupStaleTracks()
		case <-stopCh:
			return
		}
	}
}

type EventProcessor struct {
	config          *EventProcConfig
	nc              *nats.Conn
	logger          *slog.Logger
	eventSub        *nats.Subscription
	tracker         *Tracker
	alertRules      *AlertRuleManager
	alertWorkflow   *AlertWorkflowManager
	ruleEngine      *RuleEngine
	tourScheduler   *TourScheduler
	peopleCounter   *PeopleCounter
	ha              *HeatmapAggregator
	intrusion       *IntrusionDetector
	loitering       *LoiteringDetector
	abandonedObject *AbandonedObjectDetector
	forensics       *ForensicsService
	db              *sqlx.DB
	adminServer     *http.Server
	stopCh          chan struct{}
}

func NewEventProcessor(ctx context.Context, config *EventProcConfig, logger *slog.Logger) (*EventProcessor, error) {
	natsCB := common.NewNATSCircuitBreaker("event-proc")
	nc, err := common.ConnectNATSWithCircuitBreaker(config.NATSURL, natsCB)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	dbCB := common.NewDBCircuitBreaker("event-proc")
	db, err := common.ConnectDBWithCircuitBreaker(ctx, "postgres", config.DBURL, dbCB)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	migrator := common.NewMigrator(db, common.GetEnv("MIGRATIONS_DIR", "/migrations"), logger)
	if err := migrator.Run(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &EventProcessor{
		config:          config,
		nc:              nc,
		logger:          logger,
		tracker:         NewTracker(0.3, 3*time.Second),
		peopleCounter:   NewPeopleCounter(db),
		alertRules:      NewAlertRuleManager(db),
		alertWorkflow:   NewAlertWorkflowManager(db, ctx, AlertWorkflowConfig{
			EscalationTimeout: 5 * time.Minute,
			EscalationWebhook: os.Getenv("ESCALATION_WEBHOOK"),
		}, logger),
		ruleEngine:      NewRuleEngine(db, logger),
		tourScheduler:   NewTourScheduler(db, logger),
		ha:              NewHeatmapAggregator(db),
		intrusion:       NewIntrusionDetector(db, logger),
		loitering:       NewLoiteringDetector(logger),
		abandonedObject: NewAbandonedObjectDetector(logger),
		forensics:       NewForensicsService(db, logger),
		db:              db,
		stopCh:          make(chan struct{}),
	}, nil
}

func (s *EventProcessor) Start() error {
	var err error
	s.eventSub, err = s.nc.QueueSubscribe("camera.*.events", "event-proc", s.handleCameraEvent)
	if err != nil {
		return fmt.Errorf("failed to subscribe to camera events: %w", err)
	}
	s.eventSub.SetPendingLimits(1024, 64*1024*1024)

	go s.tracker.StartCleanupLoop(s.stopCh)

	s.intrusion.Start()
	s.loitering.Start()
	s.abandonedObject.Start()

	if err := s.ha.Init(context.Background()); err != nil {
		return fmt.Errorf("failed to initialize heatmap aggregator: %w", err)
	}

	mux := http.NewServeMux()
	healthHandler := common.NewHealthHandler()
	healthHandler.AddDBChecker(s.db.DB, "postgres")
	healthHandler.AddNATSChecker(s.nc, "nats")
	mux.HandleFunc("/health", healthHandler.Liveness)
	mux.HandleFunc("/ready", healthHandler.Readiness)
	mux.Handle("/api/alert-rules", common.JWTAuthMiddleware(s.adminHandler))
	mux.Handle("/api/alert-rules/", common.JWTAuthMiddleware(s.adminHandler))
	mux.Handle("/api/alerts", common.JWTAuthMiddleware(s.alertWorkflow.HandleHTTP))
	mux.Handle("/api/rules", common.JWTAuthMiddleware(s.ruleEngine.HandleHTTP))
	mux.Handle("/api/rules/", common.JWTAuthMiddleware(s.ruleEngine.HandleHTTP))
	mux.Handle("/api/tours", common.JWTAuthMiddleware(s.tourScheduler.HandleHTTP))
	mux.Handle("/api/tours/", common.JWTAuthMiddleware(s.tourScheduler.HandleHTTP))
	mux.Handle("/api/analytics/heatmap", common.JWTAuthMiddleware(handleHeatmap(s.ha)))
	mux.Handle("/api/analytics/people-counts", common.JWTAuthMiddleware(handlePeopleCounts(s.db)))
	mux.Handle("/api/events", common.JWTAuthMiddleware(s.handleEvents))
	mux.Handle("/api/events/stats", common.JWTAuthMiddleware(s.handleEventStats))
	mux.Handle("/api/incidents", common.JWTAuthMiddleware(handleIncidents(s.db)))
	mux.Handle("/api/incidents/", common.JWTAuthMiddleware(handleIncidentByID(s.db)))
	mux.Handle("/api/intrusion-zones", common.JWTAuthMiddleware(s.intrusion.HandleHTTP))
	mux.Handle("/api/intrusion-zones/", common.JWTAuthMiddleware(s.intrusion.HandleHTTP))
	mux.Handle("/api/loitering-zones", common.JWTAuthMiddleware(s.loitering.HandleHTTP))
	mux.Handle("/api/loitering-zones/", common.JWTAuthMiddleware(s.loitering.HandleHTTP))
	mux.Handle("/api/abandoned-object-zones", common.JWTAuthMiddleware(s.abandonedObject.HandleHTTP))
	mux.Handle("/api/abandoned-object-zones/", common.JWTAuthMiddleware(s.abandonedObject.HandleHTTP))
	mux.Handle("/api/forensics/search", common.JWTAuthMiddleware(s.forensics.HandleSearch))
	mux.Handle("/api/forensics/search/vector", common.JWTAuthMiddleware(s.forensics.HandleVectorSearch))
	mux.Handle("/api/forensics/tracks/", common.JWTAuthMiddleware(s.forensics.HandleTrackPath))
	mux.Handle("/api/forensics/export", common.JWTAuthMiddleware(s.forensics.HandleExport))

	s.adminServer = &http.Server{
		Addr:    s.config.AlertAdminPort,
		Handler: common.RecoveryMiddleware(mux),
	}

	go func() {
		s.logger.Info("Starting alert admin server", "address", s.config.AlertAdminPort)
		if err := s.adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Alert admin server error", "error", err)
		}
	}()

	s.logger.Info("Event Processing Service started",
		"person_confidence_threshold", s.config.PersonConfidenceThreshold)
	return nil
}

func (s *EventProcessor) handleCameraEvent(msg *nats.Msg) {
	var detections []Detection
	if err := json.Unmarshal(msg.Data, &detections); err != nil {
		s.logger.Error("Failed to unmarshal detections", "error", err)
		return
	}

	cameraID := extractCameraID(msg.Subject)

	tracks := s.tracker.matchDetections(cameraID, detections)

	if len(tracks) > 0 {
		trackData, err := json.Marshal(tracks)
		if err != nil {
			s.logger.Error("Failed to marshal tracks", "error", err)
		} else {
			trackSubject := fmt.Sprintf("camera.%s.tracks", cameraID)
			if err := s.nc.Publish(trackSubject, trackData); err != nil {
				s.logger.Error("Failed to publish tracks", "error", err)
			}
		}
	}

	now := time.Now()

	for _, d := range detections {
		if s.shouldTriggerNotification(d) {
			s.triggerNotification(msg.Subject, d)
		}

		if len(d.BBox) >= 4 {
			s.ha.RecordDetection(cameraID, [4]float64{d.BBox[0], d.BBox[1], d.BBox[2], d.BBox[3]}, now)

			center := [2]float64{
				(d.BBox[0] + d.BBox[2]) / 2,
				(d.BBox[1] + d.BBox[3]) / 2,
			}
			matches := s.alertRules.GetMatching(cameraID, d.Label, d.Confidence, center)
			for _, rule := range matches {
				s.triggerAlert(rule, d, cameraID)
				s.alertWorkflow.CreateAlert(rule.ID, cameraID, fmt.Sprintf("Rule '%s' triggered on %s", rule.Name, cameraID))
			}
		}
	}

	// Evaluate intrusion detection
	intrusionEvents := s.intrusion.Evaluate(cameraID, tracks)
	for _, ev := range intrusionEvents {
		s.logger.Info("Intrusion detected",
			"zone", ev.ZoneID, "camera", ev.CameraID,
			"direction", ev.Direction, "track", ev.TrackID)
	}

	// Evaluate loitering detection
	loiteringZones := s.loitering.zoneManager.GetActive(cameraID)
	if len(loiteringZones) > 0 {
		loiteringEvents := s.loitering.Evaluate(cameraID, tracks, loiteringZones)
		for _, ev := range loiteringEvents {
			s.logger.Info("Loitering detected",
				"zone", ev.ZoneID, "camera", ev.CameraID,
				"track", ev.TrackID, "dwell", ev.DwellSeconds)
		}
	}

	// Evaluate abandoned object detection
	s.abandonedObject.Evaluate(cameraID, detections)

	for _, d := range detections {
		// Evaluate rule engine
		eventData := map[string]interface{}{
			"camera_id":   cameraID,
			"object_type": d.Label,
			"confidence":  d.Confidence,
			"bbox":        d.BBox,
		}
		actions := s.ruleEngine.Evaluate(eventData)
		for _, action := range actions {
			s.executeAction(action, cameraID, eventData)
		}
	}
}

func (s *EventProcessor) triggerAlert(rule *AlertRule, detection Detection, cameraID string) {
	alert := map[string]interface{}{
		"rule_id":     rule.ID,
		"rule_name":   rule.Name,
		"camera_id":   cameraID,
		"object_type": detection.Label,
		"confidence":  detection.Confidence,
		"bbox":        detection.BBox,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(alert)
	if err != nil {
		s.logger.Error("Failed to marshal alert", "error", err)
		return
	}

	if err := s.nc.Publish("alerts.triggered", data); err != nil {
		s.logger.Error("Failed to publish alert", "error", err)
	}

	s.logger.Info("Alert triggered",
		"rule", rule.Name,
		"camera", cameraID,
		"object", detection.Label,
		"confidence", detection.Confidence)
}

func (s *EventProcessor) shouldTriggerNotification(d Detection) bool {
	return d.Label == "person" && d.Confidence > s.config.PersonConfidenceThreshold
}

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

func (s *EventProcessor) executeAction(action Action, cameraID string, eventData map[string]interface{}) {
	switch action.Type {
	case "webhook":
		if err := common.ValidateWebhookURL(action.Target); err != nil {
			s.logger.Error("Webhook target validation failed", "target", action.Target, "error", err)
			return
		}
		body, _ := json.Marshal(eventData)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(action.Target, "application/json", bytes.NewReader(body))
		if err != nil {
			s.logger.Error("Webhook call failed", "target", action.Target, "error", err)
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	case "alert":
		msg := action.Params["message"]
		if msg == "" {
			msg = fmt.Sprintf("Rule action triggered on %s", cameraID)
		}
		s.alertWorkflow.CreateAlert("rule", cameraID, msg)
	}
}

func extractCameraID(subject string) string {
	parts := strings.Split(subject, ".")
	if len(parts) >= 2 {
		return parts[1]
	}
	return "unknown"
}

func (s *EventProcessor) adminHandler(w http.ResponseWriter, r *http.Request) {
	id := ""
	path := r.URL.Path
	if strings.HasPrefix(path, "/api/alert-rules/") {
		id = path[len("/api/alert-rules/"):]
	}

	switch r.Method {
	case http.MethodGet:
		if id == "" {
			s.listAlertRules(w, r)
		} else {
			http.NotFound(w, r)
		}
	case http.MethodPost:
		if id == "" {
			s.createAlertRule(w, r)
		} else {
			http.NotFound(w, r)
		}
	case http.MethodPut:
		if id != "" {
			s.updateAlertRule(w, r, id)
		} else {
			http.NotFound(w, r)
		}
	case http.MethodDelete:
		if id != "" {
			s.deleteAlertRule(w, r, id)
		} else {
			http.NotFound(w, r)
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (s *EventProcessor) listAlertRules(w http.ResponseWriter, r *http.Request) {
	cameraID := r.URL.Query().Get("camera_id")
	rules := s.alertRules.List(cameraID)
	if rules == nil {
		rules = []*AlertRule{}
	}
	writeJSON(w, http.StatusOK, rules)
}

func (s *EventProcessor) createAlertRule(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var req struct {
		CameraID      string  `json:"camera_id"`
		Name          string  `json:"name"`
		ObjectType    string  `json:"object_type"`
		Zone          string  `json:"zone"`
		MinConfidence float64 `json:"min_confidence"`
		Action        string  `json:"action"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	rule := &AlertRule{
		CameraID:      req.CameraID,
		Name:          req.Name,
		ObjectType:    req.ObjectType,
		Zone:          req.Zone,
		MinConfidence: req.MinConfidence,
		Action:        req.Action,
		Enabled:       true,
	}

	if err := s.alertRules.Create(rule); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, rule)
}

func (s *EventProcessor) updateAlertRule(w http.ResponseWriter, r *http.Request, id string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var req struct {
		Name          string  `json:"name"`
		ObjectType    string  `json:"object_type"`
		Zone          string  `json:"zone"`
		MinConfidence float64 `json:"min_confidence"`
		Action        string  `json:"action"`
		Enabled       bool    `json:"enabled"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	rule := &AlertRule{
		ID:            id,
		Name:          req.Name,
		ObjectType:    req.ObjectType,
		Zone:          req.Zone,
		MinConfidence: req.MinConfidence,
		Action:        req.Action,
		Enabled:       req.Enabled,
	}

	if err := s.alertRules.Update(rule); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, rule)
}

func (s *EventProcessor) deleteAlertRule(w http.ResponseWriter, r *http.Request, id string) {
	if !s.alertRules.Delete(id) {
		writeError(w, http.StatusNotFound, "alert rule not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *EventProcessor) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	q := r.URL.Query()
	cameraID := q.Get("camera_id")
	eventType := q.Get("event_type")
	startTime := q.Get("start_time")
	endTime := q.Get("end_time")
	limit := 100
	offset := 0

	if l := q.Get("limit"); l != "" {
		if v, err := parseInt(l); err == nil && v > 0 && v <= 1000 {
			limit = v
		}
	}
	if o := q.Get("offset"); o != "" {
		if v, err := parseInt(o); err == nil && v >= 0 {
			offset = v
		}
	}

	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if cameraID != "" {
		where += fmt.Sprintf(" AND camera_id = $%d", argIdx)
		args = append(args, cameraID)
		argIdx++
	}
	if eventType != "" {
		where += fmt.Sprintf(" AND event_type = $%d", argIdx)
		args = append(args, eventType)
		argIdx++
	}
	if startTime != "" {
		where += fmt.Sprintf(" AND event_time >= $%d", argIdx)
		args = append(args, startTime)
		argIdx++
	}
	if endTime != "" {
		where += fmt.Sprintf(" AND event_time <= $%d", argIdx)
		args = append(args, endTime)
		argIdx++
	}

	var total int
	if err := s.db.Get(&total, "SELECT COUNT(*) FROM onvif_events "+where, args...); err != nil {
		s.logger.Error("failed to count onvif events", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to query events")
		return
	}

	query := "SELECT id, camera_id, subscription_id, topic, source, event_type, severity, message, event_time, created_at FROM onvif_events " +
		where + " ORDER BY event_time DESC LIMIT $"+fmt.Sprintf("%d", argIdx) + " OFFSET $" + fmt.Sprintf("%d", argIdx+1)
	args = append(args, limit, offset)

	var events []OnvifEventRow
	if err := s.db.Select(&events, query, args...); err != nil {
		s.logger.Error("failed to query onvif events", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to query events")
		return
	}
	if events == nil {
		events = []OnvifEventRow{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"events": events,
		"total":  total,
	})
}

func (s *EventProcessor) handleEventStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	q := r.URL.Query()
	cameraID := q.Get("camera_id")
	startTime := q.Get("start_time")
	endTime := q.Get("end_time")

	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if cameraID != "" {
		where += fmt.Sprintf(" AND camera_id = $%d", argIdx)
		args = append(args, cameraID)
		argIdx++
	}
	if startTime != "" {
		where += fmt.Sprintf(" AND event_time >= $%d", argIdx)
		args = append(args, startTime)
		argIdx++
	}
	if endTime != "" {
		where += fmt.Sprintf(" AND event_time <= $%d", argIdx)
		args = append(args, endTime)
		argIdx++
	}

	var total int
	if err := s.db.Get(&total, "SELECT COUNT(*) FROM onvif_events "+where, args...); err != nil {
		s.logger.Error("failed to count onvif events", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to query stats")
		return
	}

	type typeCount struct {
		EventType string `db:"event_type"`
		Count     int    `db:"count"`
	}
	var byType []typeCount
	if err := s.db.Select(&byType, "SELECT event_type, COUNT(*) as count FROM onvif_events "+where+" GROUP BY event_type ORDER BY count DESC", args...); err != nil {
		s.logger.Error("failed to query event type stats", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to query stats")
		return
	}

	byTypeMap := make(map[string]int)
	for _, tc := range byType {
		byTypeMap[tc.EventType] = tc.Count
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total":   total,
		"by_type": byTypeMap,
	})
}

func parseInt(s string) (int, error) {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}

func pointInPolygon(pt [2]float64, polygon [][2]float64) bool {
	n := len(polygon)
	inside := false
	j := n - 1
	for i := 0; i < n; i++ {
		if ((polygon[i][1] > pt[1]) != (polygon[j][1] > pt[1])) &&
			(pt[0] < (polygon[j][0]-polygon[i][0])*(pt[1]-polygon[i][1])/(polygon[j][1]-polygon[i][1])+polygon[i][0]) {
			inside = !inside
		}
		j = i
	}
	return inside
}

func (s *EventProcessor) Close() error {
	close(s.stopCh)
	s.intrusion.Stop()
	s.loitering.Stop()
	s.abandonedObject.Stop()

	var errs []error

	if s.eventSub != nil {
		if err := s.eventSub.Unsubscribe(); err != nil {
			errs = append(errs, fmt.Errorf("failed to unsubscribe: %w", err))
		}
	}

	if s.adminServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.adminServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown admin server: %w", err))
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
	logger := common.NewLogger("event-proc")
	slog.SetDefault(logger)

	common.CheckJWTSecret()

	if err := common.InitTelemetry("event-proc"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultEventProcConfig()

	common.StartMetricsServer(common.GetEnv("METRICS_ADDR", ":2112"))
	common.StartResourceMonitor(ctx)

	service, err := NewEventProcessor(ctx, config, logger)
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
