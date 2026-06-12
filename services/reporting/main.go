package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"

	"github.com/dam-vms/dam/pkg/common"
)

type ReportConfig struct {
	ID         string    `json:"id" db:"id"`
	Name       string    `json:"name" db:"name"`
	Type       string    `json:"type" db:"type"`
	Schedule   string    `json:"schedule" db:"schedule"`
	Format     string    `json:"format" db:"format"`
	Recipients []string  `json:"recipients" db:"recipients"`
	Enabled    bool      `json:"enabled" db:"enabled"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}

type ReportArchive struct {
	ID          string    `json:"id" db:"id"`
	ConfigID    string    `json:"config_id" db:"config_id"`
	Type        string    `json:"type" db:"type"`
	Format      string    `json:"format" db:"format"`
	FilePath    string    `json:"file_path" db:"file_path"`
	GeneratedAt time.Time `json:"generated_at" db:"generated_at"`
}

type ReportService struct {
	config   *ReportServiceConfig
	nc       *nats.Conn
	db       *sqlx.DB
	logger   *slog.Logger
	httpSrv  *http.Server
	schedule *nats.Subscription
}

type ReportServiceConfig struct {
	NATSURL string
	DBURL   string
	Port    string
}

func DefaultReportServiceConfig() *ReportServiceConfig {
	return &ReportServiceConfig{
		NATSURL: common.GetEnv("NATS_URL", "nats://nats:4222"),
		DBURL:   os.Getenv("DB_URL"),
		Port:    common.GetEnv("REPORTING_PORT", ":8098"),
	}
}

func NewReportService(config *ReportServiceConfig, logger *slog.Logger) (*ReportService, error) {
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
		cb := common.NewDBCircuitBreaker("reporting")
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

	if db != nil {
		mux.Handle("/api/reports", common.JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				listReportConfigs(db, w, r)
			case http.MethodPost:
				createReportConfig(db, w, r)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		}))

		mux.Handle("/api/reports/", common.JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
			id := strings.TrimPrefix(r.URL.Path, "/api/reports/")
			parts := strings.SplitN(id, "/", 2)
			id = parts[0]
			if id == "" {
				http.Error(w, "report id required", http.StatusBadRequest)
				return
			}
			action := ""
			if len(parts) > 1 {
				action = parts[1]
			}

			switch {
			case action == "generate":
				generateReport(db, w, r, id)
			case action == "download":
				downloadReport(db, w, r, id)
			case action == "" && r.Method == http.MethodGet:
				getReportConfig(db, w, r, id)
			case action == "" && r.Method == http.MethodDelete:
				deleteReportConfig(db, w, r, id)
			default:
				http.Error(w, "not found", http.StatusNotFound)
			}
		}))
	}

	return &ReportService{
		config:  config,
		nc:      nc,
		db:      db,
		logger:  logger,
		httpSrv: &http.Server{Addr: config.Port, Handler: common.RecoveryMiddleware(mux)},
	}, nil
}

func (s *ReportService) Start() error {
	if s.nc != nil {
		var err error
		s.schedule, err = s.nc.QueueSubscribe("reports.schedule", "reporting", s.handleScheduledReport)
		if err != nil {
			return fmt.Errorf("failed to subscribe to reports.schedule: %w", err)
		}
		s.schedule.SetPendingLimits(1024, 64*1024*1024)
	}
	s.logger.Info("Reporting Service started")
	return nil
}

func (s *ReportService) handleScheduledReport(msg *nats.Msg) {
	var event struct {
		ConfigID string `json:"config_id"`
	}
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		s.logger.Error("failed to unmarshal scheduled event", "error", err)
		return
	}
	if s.db == nil {
		s.logger.Error("database not configured, cannot generate scheduled report")
		return
	}
	_, err := generateReportContent(s.db, s.logger, event.ConfigID)
	if err != nil {
		s.logger.Error("scheduled report generation failed", "config_id", event.ConfigID, "error", err)
	}
}

func (s *ReportService) Close() error {
	var errs []error

	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpSrv.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown http server: %w", err))
		}
	}

	if s.schedule != nil {
		if err := s.schedule.Unsubscribe(); err != nil {
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

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func listReportConfigs(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	var configs []ReportConfig
	if err := db.Select(&configs, "SELECT id, name, type, schedule, format, recipients, enabled, created_at, updated_at FROM report_configs ORDER BY created_at DESC"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list report configs"})
		return
	}
	if configs == nil {
		configs = []ReportConfig{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"reports": configs})
}

func createReportConfig(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string   `json:"name"`
		Type       string   `json:"type"`
		Schedule   string   `json:"schedule"`
		Format     string   `json:"format"`
		Recipients []string `json:"recipients"`
		Enabled    bool     `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Name == "" || req.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and type are required"})
		return
	}

	validTypes := map[string]bool{"audit": true, "events": true, "storage": true, "health": true}
	if !validTypes[req.Type] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid type, must be one of: audit, events, storage, health"})
		return
	}

	recipientsJSON, _ := json.Marshal(req.Recipients)
	if req.Recipients == nil {
		recipientsJSON = []byte("[]")
	}
	if req.Schedule == "" {
		req.Schedule = "daily"
	}
	if req.Format == "" {
		req.Format = "pdf"
	}

	id := uuid.New().String()
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO report_configs (id, name, type, schedule, format, recipients, enabled, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)`,
		id, req.Name, req.Type, req.Schedule, req.Format, string(recipientsJSON), req.Enabled, now,
	); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create report config"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id, "status": "created"})
}

func getReportConfig(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	var config ReportConfig
	if err := db.Get(&config, "SELECT id, name, type, schedule, format, recipients, enabled, created_at, updated_at FROM report_configs WHERE id = $1", id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "report config not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"report": config})
}

func deleteReportConfig(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	result, err := db.Exec("DELETE FROM report_configs WHERE id = $1", id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete report config"})
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "report config not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func generateReport(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	archiveID, err := generateReportContent(db, slog.Default(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"archive_id": archiveID, "status": "generated"})
}

func generateReportContent(db *sqlx.DB, logger *slog.Logger, configID string) (string, error) {
	var config ReportConfig
	if err := db.Get(&config, "SELECT id, name, type, schedule, format, recipients, enabled, created_at, updated_at FROM report_configs WHERE id = $1", configID); err != nil {
		return "", fmt.Errorf("report config not found: %w", err)
	}

	var data interface{}
	var err error
	switch config.Type {
	case "audit":
		data, err = fetchAuditData(db)
	case "events":
		data, err = fetchEventSummary(db)
	case "storage":
		data, err = fetchStorageUsage(db)
	case "health":
		data, err = fetchSystemHealth(db)
	default:
		return "", fmt.Errorf("unknown report type: %s", config.Type)
	}
	if err != nil {
		return "", fmt.Errorf("failed to fetch report data: %w", err)
	}

	reportContent, err := renderReport(config, data)
	if err != nil {
		return "", fmt.Errorf("failed to render report: %w", err)
	}

	reportsDir := "/reports"
	if err := os.MkdirAll(reportsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create reports directory: %w", err)
	}

	archiveID := uuid.New().String()
	filename := fmt.Sprintf("%s_%s.%s", config.Type, time.Now().Format("20060102150405"), config.Format)
	filePath := filepath.Join(reportsDir, filename)
	if err := os.WriteFile(filePath, []byte(reportContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write report file: %w", err)
	}

	if _, err := db.Exec(
		`INSERT INTO report_archive (id, config_id, type, format, file_path, generated_at) VALUES ($1, $2, $3, $4, $5, NOW())`,
		archiveID, configID, config.Type, config.Format, filePath,
	); err != nil {
		return "", fmt.Errorf("failed to archive report: %w", err)
	}

	logger.Info("report generated", "config_id", configID, "archive_id", archiveID, "file", filePath)
	return archiveID, nil
}

func renderReport(config ReportConfig, data interface{}) (string, error) {
	tmpl := `<!DOCTYPE html>
<html>
<head><style>
body { font-family: Arial, sans-serif; margin: 40px; color: #333; }
h1 { color: #1a56db; border-bottom: 2px solid #1a56db; padding-bottom: 10px; }
h2 { color: #374151; margin-top: 30px; }
table { width: 100%; border-collapse: collapse; margin: 15px 0; }
th, td { padding: 10px; text-align: left; border-bottom: 1px solid #e5e7eb; }
th { background-color: #f3f4f6; font-weight: 600; }
.meta { color: #6b7280; font-size: 14px; margin-bottom: 20px; }
.footer { margin-top: 40px; padding-top: 20px; border-top: 1px solid #e5e7eb; font-size: 12px; color: #9ca3af; }
</style></head>
<body>
<h1>{{.Title}} - {{.Type}}</h1>
<div class="meta">Generated: {{.GeneratedAt}}</div>
<div class="meta">Report: {{.Name}}</div>
{{.Body}}
<div class="footer">EVMS Reporting Engine - Auto-generated report</div>
</body>
</html>`

	t, err := template.New("report").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	bodyHTML := renderDataTable(config.Type, data)

	vars := map[string]interface{}{
		"Title":       config.Name,
		"Type":        strings.ToUpper(config.Type[:1]) + config.Type[1:] + " Report",
		"Name":        config.Name,
		"GeneratedAt": time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		"Body":        template.HTML(bodyHTML),
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

func renderDataTable(reportType string, data interface{}) string {
	var buf bytes.Buffer
	switch reportType {
	case "audit":
		entries, ok := data.([]AuditEntry)
		if !ok {
			return "<p>No data available</p>"
		}
		buf.WriteString("<table><thead><tr><th>Time</th><th>Actor</th><th>Action</th><th>Resource</th><th>Status</th></tr></thead><tbody>")
		for _, e := range entries {
			buf.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>",
				e.Timestamp, e.Actor, e.Action, e.Resource, e.Status))
		}
		buf.WriteString("</tbody></table>")
	case "events":
		entries, ok := data.([]EventSummary)
		if !ok {
			return "<p>No data available</p>"
		}
		buf.WriteString("<table><thead><tr><th>Camera</th><th>Event Type</th><th>Count</th><th>Last Occurrence</th></tr></thead><tbody>")
		for _, e := range entries {
			buf.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%d</td><td>%s</td></tr>",
				e.CameraID, e.ObjectType, e.Count, e.LastTime))
		}
		buf.WriteString("</tbody></table>")
	case "storage":
		entries, ok := data.([]StorageEntry)
		if !ok {
			return "<p>No data available</p>"
		}
		buf.WriteString("<table><thead><tr><th>Camera</th><th>Total Size</th><th>Recording Days</th><th>Oldest</th><th>Latest</th></tr></thead><tbody>")
		for _, e := range entries {
			buf.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%d</td><td>%s</td><td>%s</td></tr>",
				e.CameraID, e.TotalSize, e.RecordingDays, e.Oldest, e.Latest))
		}
		buf.WriteString("</tbody></table>")
	case "health":
		entries, ok := data.([]HealthEntry)
		if !ok {
			return "<p>No data available</p>"
		}
		buf.WriteString("<table><thead><tr><th>Service</th><th>Status</th><th>Uptime</th><th>Last Check</th></tr></thead><tbody>")
		for _, e := range entries {
			buf.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>",
				e.Service, e.Status, e.Uptime, e.LastCheck))
		}
		buf.WriteString("</tbody></table>")
	}
	return buf.String()
}

type AuditEntry struct {
	Timestamp string `db:"timestamp"`
	Actor     string `db:"actor"`
	Action    string `db:"action"`
	Resource  string `db:"resource"`
	Status    string `db:"status"`
}

type EventSummary struct {
	CameraID   string `db:"camera_id"`
	ObjectType string `db:"object_type"`
	Count      int    `db:"count"`
	LastTime   string `db:"last_time"`
}

type StorageEntry struct {
	CameraID       string `db:"camera_id"`
	TotalSize      string `db:"total_size"`
	RecordingDays  int    `db:"recording_days"`
	Oldest         string `db:"oldest"`
	Latest         string `db:"latest"`
}

type HealthEntry struct {
	Service   string `db:"service"`
	Status    string `db:"status"`
	Uptime    string `db:"uptime"`
	LastCheck string `db:"last_check"`
}

func fetchAuditData(db *sqlx.DB) (interface{}, error) {
	var entries []AuditEntry
	if err := db.Select(&entries, `SELECT COALESCE(actor, 'system') as actor, action, resource, status, 
		COALESCE(TO_CHAR(timestamp AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS'), '') as timestamp 
		FROM audit_log ORDER BY timestamp DESC LIMIT 100`); err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []AuditEntry{}
	}
	return entries, nil
}

func fetchEventSummary(db *sqlx.DB) (interface{}, error) {
	var entries []EventSummary
	if err := db.Select(&entries, `SELECT camera_id, object_type, COUNT(*) as count, 
		COALESCE(TO_CHAR(MAX(event_time) AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS'), '') as last_time 
		FROM ai_events GROUP BY camera_id, object_type ORDER BY count DESC LIMIT 100`); err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []EventSummary{}
	}
	return entries, nil
}

func fetchStorageUsage(db *sqlx.DB) (interface{}, error) {
	var entries []StorageEntry
	if err := db.Select(&entries, `SELECT camera_id, 
		pg_size_pretty(SUM(file_size)::bigint) as total_size, 
		COUNT(DISTINCT DATE(start_time)) as recording_days,
		COALESCE(TO_CHAR(MIN(start_time) AT TIME ZONE 'UTC', 'YYYY-MM-DD'), '') as oldest,
		COALESCE(TO_CHAR(MAX(start_time) AT TIME ZONE 'UTC', 'YYYY-MM-DD'), '') as latest
		FROM recordings GROUP BY camera_id ORDER BY SUM(file_size) DESC LIMIT 100`); err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []StorageEntry{}
	}
	return entries, nil
}

func fetchSystemHealth(db *sqlx.DB) (interface{}, error) {
	var entries []HealthEntry
	if err := db.Select(&entries, `SELECT service, status, 
		COALESCE(uptime::text, '') as uptime,
		COALESCE(TO_CHAR(last_check AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS'), '') as last_check 
		FROM system_health ORDER BY service`); err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []HealthEntry{}
	}
	return entries, nil
}

func downloadReport(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	var archive ReportArchive
	if err := db.Get(&archive, "SELECT id, config_id, type, format, file_path, generated_at FROM report_archive WHERE id = $1", id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "report archive not found"})
		return
	}

	content, err := os.ReadFile(archive.FilePath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "report file not found"})
		return
	}

	contentType := "text/html"
	switch archive.Format {
	case "csv":
		contentType = "text/csv"
	case "json":
		contentType = "application/json"
	case "pdf":
		contentType = "application/pdf"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(archive.FilePath)))
	w.WriteHeader(http.StatusOK)
	io.Copy(w, bytes.NewReader(content))
}

func main() {
	logger := common.NewLogger("reporting")
	slog.SetDefault(logger)

	common.CheckJWTSecret()

	if err := common.InitTelemetry("reporting"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultReportServiceConfig()
	common.StartMetricsServer(common.GetEnv("METRICS_ADDR", ":2112"))
	common.StartResourceMonitor(ctx)

	service, err := NewReportService(config, logger)
	if err != nil {
		logger.Error("Failed to initialize reporting service", "error", err)
		os.Exit(1)
	}
	defer service.Close()

	if err := service.Start(); err != nil {
		logger.Error("Failed to start reporting service", "error", err)
		os.Exit(1)
	}

	go func() {
		logger.Info("Reporting service listening", "addr", config.Port)
		if err := service.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", "error", err)
		}
	}()

	<-ctx.Done()
	logger.Info("Shutting down Reporting Service...")
}
