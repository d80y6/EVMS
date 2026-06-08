package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type ChannelType string

const (
	ChannelEmail ChannelType = "email"
	ChannelSMS   ChannelType = "sms"
	ChannelPush  ChannelType = "push"
)

type NotificationChannel interface {
	Type() ChannelType
	Send(recipient, subject, body string) error
}

type EmailConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	TLS      bool   `json:"tls"`
}

type EmailChannel struct {
	config EmailConfig
	logger *slog.Logger
}

func NewEmailChannel(cfg EmailConfig, logger *slog.Logger) *EmailChannel {
	return &EmailChannel{config: cfg, logger: logger}
}

func (c *EmailChannel) Type() ChannelType { return ChannelEmail }

func (c *EmailChannel) Send(recipient, subject, body string) error {
	auth := smtp.PlainAuth("", c.config.Username, c.config.Password, c.config.Host)
	addr := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)

	msg := &bytes.Buffer{}
	msg.WriteString(fmt.Sprintf("From: %s\r\n", c.config.From))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", recipient))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	if c.config.TLS {
		tlsCfg := &tls.Config{ServerName: c.config.Host}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("tls dial failed: %w", err)
		}
		client, err := smtp.NewClient(conn, c.config.Host)
		if err != nil {
			conn.Close()
			return fmt.Errorf("smtp client failed: %w", err)
		}
		defer client.Close()
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth failed: %w", err)
		}
		if err = client.Mail(c.config.From); err != nil {
			return fmt.Errorf("smtp mail from failed: %w", err)
		}
		if err = client.Rcpt(recipient); err != nil {
			return fmt.Errorf("smtp rcpt failed: %w", err)
		}
		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("smtp data failed: %w", err)
		}
		if _, err = w.Write(msg.Bytes()); err != nil {
			return fmt.Errorf("smtp write failed: %w", err)
		}
		return w.Close()
	}

	return smtp.SendMail(addr, auth, c.config.From, []string{recipient}, msg.Bytes())
}

type SMSConfig struct {
	AccountSID string `json:"account_sid"`
	AuthToken  string `json:"auth_token"`
	FromNumber string `json:"from_number"`
}

type SMSChannel struct {
	config SMSConfig
	logger *slog.Logger
	client *http.Client
}

func NewSMSChannel(cfg SMSConfig, logger *slog.Logger) *SMSChannel {
	return &SMSChannel{config: cfg, logger: logger, client: &http.Client{Timeout: 10 * time.Second}}
}

func (c *SMSChannel) Type() ChannelType { return ChannelSMS }

func (c *SMSChannel) Send(recipient, subject, body string) error {
	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", c.config.AccountSID)
	payload := fmt.Sprintf("To=%s&From=%s&Body=%s", url.QueryEscape(recipient), url.QueryEscape(c.config.FromNumber), url.QueryEscape(body))
	req, err := http.NewRequest("POST", apiURL, strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("twilio request failed: %w", err)
	}
	req.SetBasicAuth(c.config.AccountSID, c.config.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("twilio send failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("twilio returned status %d", resp.StatusCode)
	}
	return nil
}

type PushConfig struct {
	ServerKey string `json:"server_key"`
}

type PushChannel struct {
	config PushConfig
	logger *slog.Logger
	client *http.Client
}

func NewPushChannel(cfg PushConfig, logger *slog.Logger) *PushChannel {
	return &PushChannel{config: cfg, logger: logger, client: &http.Client{Timeout: 10 * time.Second}}
}

func (c *PushChannel) Type() ChannelType { return ChannelPush }

func (c *PushChannel) Send(recipient, subject, body string) error {
	payload := map[string]interface{}{
		"to": recipient,
		"notification": map[string]string{
			"title": subject,
			"body":  body,
		},
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", "https://fcm.googleapis.com/fcm/send", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("fcm request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "key="+c.config.ServerKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("fcm send failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("fcm returned status %d", resp.StatusCode)
	}
	return nil
}

type ChannelDBRow struct {
	ID              string          `db:"id" json:"id"`
	TenantID        string          `db:"tenant_id" json:"tenant_id"`
	Type            string          `db:"type" json:"type"`
	Name            string          `db:"name" json:"name"`
	Config          json.RawMessage `db:"config" json:"config"`
	Enabled         bool            `db:"enabled" json:"enabled"`
	RateLimitPerMin int             `db:"rate_limit_per_min" json:"rate_limit_per_min"`
	CreatedAt       time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time       `db:"updated_at" json:"updated_at"`
}

type NotificationTemplate struct {
	ID        string          `db:"id" json:"id"`
	Name      string          `db:"name" json:"name"`
	Subject   string          `db:"subject" json:"subject"`
	BodyHTML  string          `db:"body_html" json:"body_html"`
	BodyText  string          `db:"body_text" json:"body_text"`
	Variables json.RawMessage `db:"variables" json:"variables"`
	CreatedAt time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt time.Time       `db:"updated_at" json:"updated_at"`
}

type ChannelManager struct {
	mu       sync.RWMutex
	channels map[string]NotificationChannel
	db       *sqlx.DB
	logger   *slog.Logger
}

func NewChannelManager(db *sqlx.DB, logger *slog.Logger) *ChannelManager {
	return &ChannelManager{
		channels: make(map[string]NotificationChannel),
		db:       db,
		logger:   logger,
	}
}

func (m *ChannelManager) AddChannel(id string, ch NotificationChannel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels[id] = ch
}

func (m *ChannelManager) RemoveChannel(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.channels, id)
}

func (m *ChannelManager) GetChannel(id string) (NotificationChannel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ch, ok := m.channels[id]
	return ch, ok
}

func (m *ChannelManager) LoadFromDB(ctx context.Context) error {
	if m.db == nil {
		return nil
	}
	var rows []ChannelDBRow
	if err := m.db.SelectContext(ctx, &rows, "SELECT id, tenant_id, type, name, config, enabled, rate_limit_per_min, created_at, updated_at FROM notification_channels WHERE enabled = true"); err != nil {
		return fmt.Errorf("failed to load channels: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, row := range rows {
		ch, err := m.buildChannel(row)
		if err != nil {
			m.logger.Error("failed to build channel", "id", row.ID, "error", err)
			continue
		}
		m.channels[row.ID] = ch
	}
	m.logger.Info("loaded notification channels", "count", len(m.channels))
	return nil
}

func (m *ChannelManager) buildChannel(row ChannelDBRow) (NotificationChannel, error) {
	switch ChannelType(row.Type) {
	case ChannelEmail:
		var cfg EmailConfig
		if err := json.Unmarshal(row.Config, &cfg); err != nil {
			return nil, fmt.Errorf("invalid email config: %w", err)
		}
		return NewEmailChannel(cfg, m.logger), nil
	case ChannelSMS:
		var cfg SMSConfig
		if err := json.Unmarshal(row.Config, &cfg); err != nil {
			return nil, fmt.Errorf("invalid sms config: %w", err)
		}
		return NewSMSChannel(cfg, m.logger), nil
	case ChannelPush:
		var cfg PushConfig
		if err := json.Unmarshal(row.Config, &cfg); err != nil {
			return nil, fmt.Errorf("invalid push config: %w", err)
		}
		return NewPushChannel(cfg, m.logger), nil
	default:
		return nil, fmt.Errorf("unknown channel type: %s", row.Type)
	}
}

type ChannelRateLimiter struct {
	mu       sync.Mutex
	tokens   map[string]*channelTokens
}

type channelTokens struct {
	count    float64
	last     time.Time
	rate     float64
}

func NewChannelRateLimiter() *ChannelRateLimiter {
	return &ChannelRateLimiter{
		tokens: make(map[string]*channelTokens),
	}
}

func (rl *ChannelRateLimiter) Allow(channelID string, ratePerMin int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	ct, ok := rl.tokens[channelID]
	if !ok {
		ct = &channelTokens{count: float64(ratePerMin), last: now, rate: float64(ratePerMin) / 60.0}
		rl.tokens[channelID] = ct
	}

	elapsed := now.Sub(ct.last).Seconds()
	ct.count += elapsed * ct.rate
	if ct.count > float64(ratePerMin) {
		ct.count = float64(ratePerMin)
	}
	ct.last = now

	if ct.count >= 1 {
		ct.count--
		return true
	}
	return false
}

func retryWithBackoff(maxRetries int, fn func() error) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		wait := time.Duration(math.Pow(2, float64(i))) * time.Second
		time.Sleep(wait)
	}
	return fmt.Errorf("all retries failed: %w", err)
}

var alertEmailTemplate = template.Must(template.New("alert").Parse(`<!DOCTYPE html>
<html><head><style>
body { font-family: Arial, sans-serif; background: #f4f4f4; padding: 20px; }
.container { background: white; border-radius: 8px; padding: 24px; max-width: 600px; margin: auto; }
.header { border-bottom: 2px solid #e74c3c; padding-bottom: 12px; margin-bottom: 16px; }
.header h1 { color: #e74c3c; margin: 0; font-size: 20px; }
.detail { margin: 8px 0; }
.label { font-weight: bold; color: #555; }
.footer { margin-top: 20px; font-size: 12px; color: #999; border-top: 1px solid #eee; padding-top: 12px; }
</style></head><body>
<div class="container">
<div class="header"><h1>{{.Title}}</h1></div>
<div class="detail"><span class="label">Camera:</span> {{.CameraID}}</div>
<div class="detail"><span class="label">Time:</span> {{.Timestamp}}</div>
<div class="detail"><span class="label">Message:</span> {{.Message}}</div>
<div class="detail"><span class="label">Confidence:</span> {{.Confidence}}</div>
<div class="footer">This is an automated alert from EVMS. Do not reply.</div>
</div></body></html>`))

type AlertTemplateData struct {
	Title      string
	CameraID   string
	Timestamp  string
	Message    string
	Confidence string
}

func renderAlertEmail(data AlertTemplateData) (string, error) {
	var buf bytes.Buffer
	if err := alertEmailTemplate.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

var eventEmailTemplate = template.Must(template.New("event").Parse(`<!DOCTYPE html>
<html><head><style>
body { font-family: Arial, sans-serif; background: #f4f4f4; padding: 20px; }
.container { background: white; border-radius: 8px; padding: 24px; max-width: 600px; margin: auto; }
.header { border-bottom: 2px solid #3498db; padding-bottom: 12px; margin-bottom: 16px; }
.header h1 { color: #3498db; margin: 0; font-size: 20px; }
.detail { margin: 8px 0; }
.label { font-weight: bold; color: #555; }
.footer { margin-top: 20px; font-size: 12px; color: #999; border-top: 1px solid #eee; padding-top: 12px; }
</style></head><body>
<div class="container">
<div class="header"><h1>{{.Title}}</h1></div>
<div class="detail"><span class="label">Camera:</span> {{.CameraID}}</div>
<div class="detail"><span class="label">Time:</span> {{.Timestamp}}</div>
<div class="detail"><span class="label">Object:</span> {{.ObjectType}}</div>
<div class="detail"><span class="label">Confidence:</span> {{.Confidence}}</div>
<div class="footer">This is an automated event notification from EVMS.</div>
</div></body></html>`))

type EventTemplateData struct {
	Title      string
	CameraID   string
	Timestamp  string
	ObjectType string
	Confidence string
}

func renderEventEmail(data EventTemplateData) (string, error) {
	var buf bytes.Buffer
	if err := eventEmailTemplate.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func handleChannels(db *sqlx.DB, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			listChannels(db, w, r)
		case http.MethodPost:
			createChannel(db, w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleChannelByID(db *sqlx.DB, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/channels/")
		if id == "" {
			http.Error(w, "channel id required", http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodPut:
			updateChannel(db, w, r, id)
		case http.MethodDelete:
			deleteChannel(db, w, r, id)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func listChannels(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	var channels []ChannelDBRow
	if err := db.Select(&channels, "SELECT id, tenant_id, type, name, config, enabled, rate_limit_per_min, created_at, updated_at FROM notification_channels ORDER BY created_at DESC"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list channels"})
		return
	}
	if channels == nil {
		channels = []ChannelDBRow{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"channels": channels})
}

func createChannel(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID        string          `json:"tenant_id"`
		Type            string          `json:"type"`
		Name            string          `json:"name"`
		Config          json.RawMessage `json:"config"`
		RateLimitPerMin int             `json:"rate_limit_per_min"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Name == "" || req.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and type are required"})
		return
	}
	if req.RateLimitPerMin <= 0 {
		req.RateLimitPerMin = 60
	}

	id := uuid.New().String()
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO notification_channels (id, tenant_id, type, name, config, enabled, rate_limit_per_min, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, true, $6, $7, $7)`,
		id, req.TenantID, req.Type, req.Name, req.Config, req.RateLimitPerMin, now,
	); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create channel"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id, "status": "created"})
}

func updateChannel(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Name            *string          `json:"name,omitempty"`
		Config          *json.RawMessage `json:"config,omitempty"`
		Enabled         *bool            `json:"enabled,omitempty"`
		RateLimitPerMin *int             `json:"rate_limit_per_min,omitempty"`
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
	if req.Config != nil {
		setClauses = append(setClauses, fmt.Sprintf("config = $%d", argIdx))
		args = append(args, *req.Config)
		argIdx++
	}
	if req.Enabled != nil {
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", argIdx))
		args = append(args, *req.Enabled)
		argIdx++
	}
	if req.RateLimitPerMin != nil {
		setClauses = append(setClauses, fmt.Sprintf("rate_limit_per_min = $%d", argIdx))
		args = append(args, *req.RateLimitPerMin)
		argIdx++
	}
	if len(setClauses) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	args = append(args, id)
	query := fmt.Sprintf("UPDATE notification_channels SET %s WHERE id = $%d", strings.Join(setClauses, ", "), argIdx)
	result, err := db.Exec(query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update channel"})
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "channel not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func deleteChannel(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	result, err := db.Exec("DELETE FROM notification_channels WHERE id = $1", id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete channel"})
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "channel not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func handleTemplates(db *sqlx.DB, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			listTemplates(db, w, r)
		case http.MethodPost:
			createTemplate(db, w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleTemplateByID(db *sqlx.DB, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/templates/")
		if id == "" {
			http.Error(w, "template id required", http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodPut:
			updateTemplate(db, w, r, id)
		case http.MethodDelete:
			deleteTemplate(db, w, r, id)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func listTemplates(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	var templates []NotificationTemplate
	if err := db.Select(&templates, "SELECT id, name, subject, body_html, body_text, variables, created_at, updated_at FROM notification_templates ORDER BY name ASC"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list templates"})
		return
	}
	if templates == nil {
		templates = []NotificationTemplate{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"templates": templates})
}

func createTemplate(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string          `json:"name"`
		Subject   string          `json:"subject"`
		BodyHTML  string          `json:"body_html"`
		BodyText  string          `json:"body_text"`
		Variables json.RawMessage `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if req.Variables == nil {
		req.Variables = json.RawMessage("[]")
	}

	id := uuid.New().String()
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO notification_templates (id, name, subject, body_html, body_text, variables, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $7)`,
		id, req.Name, req.Subject, req.BodyHTML, req.BodyText, req.Variables, now,
	); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create template"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id, "status": "created"})
}

func updateTemplate(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Name      *string          `json:"name,omitempty"`
		Subject   *string          `json:"subject,omitempty"`
		BodyHTML  *string          `json:"body_html,omitempty"`
		BodyText  *string          `json:"body_text,omitempty"`
		Variables *json.RawMessage `json:"variables,omitempty"`
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
	if req.Subject != nil {
		setClauses = append(setClauses, fmt.Sprintf("subject = $%d", argIdx))
		args = append(args, *req.Subject)
		argIdx++
	}
	if req.BodyHTML != nil {
		setClauses = append(setClauses, fmt.Sprintf("body_html = $%d", argIdx))
		args = append(args, *req.BodyHTML)
		argIdx++
	}
	if req.BodyText != nil {
		setClauses = append(setClauses, fmt.Sprintf("body_text = $%d", argIdx))
		args = append(args, *req.BodyText)
		argIdx++
	}
	if req.Variables != nil {
		setClauses = append(setClauses, fmt.Sprintf("variables = $%d", argIdx))
		args = append(args, *req.Variables)
		argIdx++
	}
	if len(setClauses) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	args = append(args, id)
	query := fmt.Sprintf("UPDATE notification_templates SET %s WHERE id = $%d", strings.Join(setClauses, ", "), argIdx)
	result, err := db.Exec(query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update template"})
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "template not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func deleteTemplate(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	result, err := db.Exec("DELETE FROM notification_templates WHERE id = $1", id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete template"})
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "template not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func handleNotificationLog(db *sqlx.DB, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		q := r.URL.Query()
		channelID := q.Get("channel_id")
		status := q.Get("status")
		limit := 100
		if l := q.Get("limit"); l != "" {
			fmt.Sscanf(l, "%d", &limit)
		}

		where := "WHERE 1=1"
		args := []interface{}{}
		argIdx := 1

		if channelID != "" {
			where += fmt.Sprintf(" AND channel_id = $%d", argIdx)
			args = append(args, channelID)
			argIdx++
		}
		if status != "" {
			where += fmt.Sprintf(" AND status = $%d", argIdx)
			args = append(args, status)
			argIdx++
		}

		query := fmt.Sprintf("SELECT id, channel_id, recipient, subject, status, error, sent_at, created_at FROM notification_log %s ORDER BY created_at DESC LIMIT $%d", where, argIdx)
		args = append(args, limit)

		type LogEntry struct {
			ID        string     `db:"id" json:"id"`
			ChannelID string     `db:"channel_id" json:"channel_id"`
			Recipient string     `db:"recipient" json:"recipient"`
			Subject   string     `db:"subject" json:"subject"`
			Status    string     `db:"status" json:"status"`
			Error     *string    `db:"error" json:"error,omitempty"`
			SentAt    *time.Time `db:"sent_at" json:"sent_at,omitempty"`
			CreatedAt time.Time  `db:"created_at" json:"created_at"`
		}

		var entries []LogEntry
		if err := db.Select(&entries, query, args...); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query notification log"})
			return
		}
		if entries == nil {
			entries = []LogEntry{}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"entries": entries})
	}
}
