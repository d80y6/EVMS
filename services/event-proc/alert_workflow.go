package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/jmoiron/sqlx"
)

type AlertStatus string

const (
	AlertTriggered     AlertStatus = "triggered"
	AlertAcknowledged  AlertStatus = "acknowledged"
	AlertEscalated     AlertStatus = "escalated"
	AlertResolved      AlertStatus = "resolved"
)

type Alert struct {
	ID                string      `json:"id"`
	RuleID            string      `json:"rule_id"`
	CameraID          string      `json:"camera_id"`
	Message           string      `json:"message"`
	Status            AlertStatus `json:"status"`
	CreatedAt         time.Time   `json:"created_at"`
	AckedBy           string      `json:"acked_by,omitempty"`
	AckedAt           *time.Time  `json:"acked_at,omitempty"`
	Escalated         bool        `json:"escalated"`
	EscalationWebhook string      `json:"escalation_webhook,omitempty"`
}

type AlertWorkflowManager struct {
	mu     sync.RWMutex
	alerts map[string]*Alert
	config AlertWorkflowConfig
	logger *slog.Logger
	db     *sqlx.DB
	ctx    context.Context
	cancel context.CancelFunc
}

type AlertWorkflowConfig struct {
	EscalationTimeout time.Duration `json:"escalation_timeout"`
	EscalationWebhook string        `json:"escalation_webhook"`
	CheckInterval     time.Duration `json:"check_interval"`
}

func NewAlertWorkflowManager(db *sqlx.DB, ctx context.Context, cfg AlertWorkflowConfig, logger *slog.Logger) *AlertWorkflowManager {
	if cfg.EscalationTimeout == 0 {
		cfg.EscalationTimeout = 5 * time.Minute
	}
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = 30 * time.Second
	}
	ctx, cancel := context.WithCancel(ctx)
	m := &AlertWorkflowManager{
		alerts: make(map[string]*Alert),
		config: cfg,
		logger: logger,
		db:     db,
		ctx:    ctx,
		cancel: cancel,
	}
	m.loadFromDB()
	go m.escalationLoop()
	return m
}

func (m *AlertWorkflowManager) loadFromDB() {
	if m.db == nil {
		return
	}
	var rows []struct {
		ID                string     `db:"id"`
		RuleID            string     `db:"rule_id"`
		CameraID          string     `db:"camera_id"`
		Message           string     `db:"message"`
		Status            string     `db:"status"`
		AckedBy           string     `db:"acked_by"`
		AckedAt           *time.Time `db:"acked_at"`
		Escalated         bool       `db:"escalated"`
		EscalationWebhook string     `db:"escalation_webhook"`
		CreatedAt         time.Time  `db:"created_at"`
	}
	if err := m.db.Select(&rows, "SELECT id, rule_id, camera_id, message, status, acked_by, acked_at, escalated, escalation_webhook, created_at FROM alerts"); err != nil {
		m.logger.Warn("Failed to load alerts from DB", "error", err)
		return
	}
	for _, row := range rows {
		alert := &Alert{
			ID:                row.ID,
			RuleID:            row.RuleID,
			CameraID:          row.CameraID,
			Message:           row.Message,
			Status:            AlertStatus(row.Status),
			CreatedAt:         row.CreatedAt,
			AckedBy:           row.AckedBy,
			AckedAt:           row.AckedAt,
			Escalated:         row.Escalated,
			EscalationWebhook: row.EscalationWebhook,
		}
		m.alerts[alert.ID] = alert
	}
	m.logger.Info("Loaded alerts from database", "count", len(rows))
}

func (m *AlertWorkflowManager) saveAlert(alert *Alert) error {
	if m.db == nil {
		return nil
	}
	_, err := m.db.Exec(`INSERT INTO alerts (id, rule_id, camera_id, message, status, acked_by, acked_at, escalated, escalation_webhook, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (id) DO UPDATE SET status=$5, acked_by=$6, acked_at=$7, escalated=$8, escalation_webhook=$9, updated_at=NOW()`,
		alert.ID, alert.RuleID, alert.CameraID, alert.Message, string(alert.Status), alert.AckedBy, alert.AckedAt, alert.Escalated, alert.EscalationWebhook, alert.CreatedAt)
	return err
}

func (m *AlertWorkflowManager) CreateAlert(ruleID, cameraID, message string) *Alert {
	m.mu.Lock()
	defer m.mu.Unlock()
	alert := &Alert{
		ID:        fmt.Sprintf("alert_%d", time.Now().UnixNano()),
		RuleID:    ruleID,
		CameraID:  cameraID,
		Message:   message,
		Status:    AlertTriggered,
		CreatedAt: time.Now(),
	}
	m.alerts[alert.ID] = alert
	if err := m.saveAlert(alert); err != nil {
		m.logger.Error("Failed to persist alert", "id", alert.ID, "error", err)
	}
	m.logger.Info("Alert created", "id", alert.ID, "camera", cameraID)
	return alert
}

func (m *AlertWorkflowManager) Acknowledge(id, username string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	alert, ok := m.alerts[id]
	if !ok {
		return fmt.Errorf("alert not found")
	}
	now := time.Now()
	alert.Status = AlertAcknowledged
	alert.AckedBy = username
	alert.AckedAt = &now
	if err := m.saveAlert(alert); err != nil {
		m.logger.Error("Failed to persist alert acknowledgment", "id", id, "error", err)
	}
	m.logger.Info("Alert acknowledged", "id", id, "by", username)
	return nil
}

func (m *AlertWorkflowManager) escalationLoop() {
	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.mu.Lock()
			now := time.Now()
			for _, alert := range m.alerts {
				if alert.Status == AlertTriggered && now.Sub(alert.CreatedAt) > m.config.EscalationTimeout {
					alert.Status = AlertEscalated
					alert.Escalated = true
					if err := m.saveAlert(alert); err != nil {
						m.logger.Error("Failed to persist escalation", "id", alert.ID, "error", err)
					}
					m.logger.Warn("Alert escalated", "id", alert.ID)
					if m.config.EscalationWebhook != "" {
						go m.fireEscalationWebhook(alert)
					}
				}
			}
			m.mu.Unlock()
		}
	}
}

func (m *AlertWorkflowManager) fireEscalationWebhook(alert *Alert) {
	if err := common.ValidateWebhookURL(m.config.EscalationWebhook); err != nil {
		m.logger.Error("Escalation webhook validation failed", "error", err)
		return
	}
	body, _ := json.Marshal(alert)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(m.config.EscalationWebhook, "application/json", bytes.NewReader(body))
	if err != nil {
		m.logger.Error("Escalation webhook failed", "error", err)
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func (m *AlertWorkflowManager) listAlerts(ctx context.Context) []*Alert {
	m.mu.RLock()
	defer m.mu.RUnlock()
	alerts := make([]*Alert, 0, len(m.alerts))
	for _, a := range m.alerts {
		alerts = append(alerts, a)
	}
	return alerts
}

func (m *AlertWorkflowManager) HandleHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		alerts := m.listAlerts(r.Context())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"alerts": alerts})
	case http.MethodPost:
		var req struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request")
			return
		}
		if err := m.Acknowledge(req.ID, req.Username); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "acknowledged"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
