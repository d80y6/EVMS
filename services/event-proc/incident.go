package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type IncidentSeverity string

const (
	SeverityLow      IncidentSeverity = "low"
	SeverityMedium   IncidentSeverity = "medium"
	SeverityHigh     IncidentSeverity = "high"
	SeverityCritical IncidentSeverity = "critical"
)

type IncidentStatus string

const (
	IncidentOpen         IncidentStatus = "open"
	IncidentInvestigating IncidentStatus = "investigating"
	IncidentResolved     IncidentStatus = "resolved"
	IncidentClosed       IncidentStatus = "closed"
)

type Incident struct {
	ID          string          `db:"id" json:"id"`
	TenantID    string          `db:"tenant_id" json:"tenant_id"`
	Title       string          `db:"title" json:"title"`
	Description string          `db:"description" json:"description"`
	Severity    string          `db:"severity" json:"severity"`
	Status      string          `db:"status" json:"status"`
	AssignedTo  string          `db:"assigned_to" json:"assigned_to"`
	Timeline    json.RawMessage `db:"timeline" json:"timeline"`
	CreatedAt   time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time       `db:"updated_at" json:"updated_at"`
}

type IncidentNote struct {
	ID         string    `db:"id" json:"id"`
	IncidentID string    `db:"incident_id" json:"incident_id"`
	Author     string    `db:"author" json:"author"`
	Body       string    `db:"body" json:"body"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
}

type TimelineEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail"`
}

type IncidentManager struct {
	db     *sqlx.DB
}

func NewIncidentManager(db *sqlx.DB) *IncidentManager {
	return &IncidentManager{db: db}
}

func (m *IncidentManager) CreateFromAlert(ruleID, cameraID, message, severity, tenantID string) (*Incident, error) {
	title := fmt.Sprintf("Alert: %s on camera %s", message, cameraID)
	timeline := []TimelineEntry{
		{Timestamp: time.Now().UTC(), Actor: "system", Action: "created", Detail: fmt.Sprintf("Incident created from alert (rule: %s, camera: %s)", ruleID, cameraID)},
	}
	timelineJSON, _ := json.Marshal(timeline)

	id := uuid.New().String()
	now := time.Now().UTC()
	_, err := m.db.Exec(
		`INSERT INTO incidents (id, tenant_id, title, description, severity, status, assigned_to, timeline, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, 'open', '', $6, $7, $7)`,
		id, tenantID, title, message, severity, timelineJSON, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create incident from alert: %w", err)
	}

	_, err = m.db.Exec(`INSERT INTO incident_events (incident_id, event_id) VALUES ($1, $2)`, id, ruleID)
	if err != nil {
		return nil, fmt.Errorf("failed to link incident event: %w", err)
	}

	return m.GetByID(id)
}

func (m *IncidentManager) GetByID(id string) (*Incident, error) {
	var inc Incident
	if err := m.db.Get(&inc, "SELECT id, tenant_id, title, description, severity, status, assigned_to, timeline, created_at, updated_at FROM incidents WHERE id = $1", id); err != nil {
		return nil, err
	}
	return &inc, nil
}

func (m *IncidentManager) addTimelineEntry(id, actor, action, detail string) {
	var existing json.RawMessage
	if err := m.db.Get(&existing, "SELECT timeline FROM incidents WHERE id = $1", id); err != nil {
		return
	}

	var entries []TimelineEntry
	json.Unmarshal(existing, &entries)
	entries = append(entries, TimelineEntry{
		Timestamp: time.Now().UTC(),
		Actor:     actor,
		Action:    action,
		Detail:    detail,
	})
	updated, _ := json.Marshal(entries)
	m.db.Exec("UPDATE incidents SET timeline = $1, updated_at = NOW() WHERE id = $2", updated, id)
}

func handleIncidents(db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			writeError(w, http.StatusInternalServerError, "database not configured")
			return
		}
		switch r.Method {
		case http.MethodGet:
			listIncidents(db, w, r)
		case http.MethodPost:
			createIncident(db, w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleIncidentByID(db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			writeError(w, http.StatusInternalServerError, "database not configured")
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/incidents/")
		parts := strings.SplitN(path, "/", 2)
		id := parts[0]
		if id == "" {
			http.Error(w, "incident id required", http.StatusBadRequest)
			return
		}

		if len(parts) == 2 {
			switch parts[1] {
			case "notes":
				if r.Method == http.MethodGet {
					listIncidentNotes(db, w, r, id)
				} else if r.Method == http.MethodPost {
					addIncidentNote(db, w, r, id)
				} else {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				}
				return
			case "escalate":
				if r.Method == http.MethodPost {
					escalateIncident(db, w, r, id)
				} else {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				}
				return
			}
		}

		switch r.Method {
		case http.MethodGet:
			getIncident(db, w, r, id)
		case http.MethodPut:
			updateIncident(db, w, r, id)
		case http.MethodDelete:
			deleteIncident(db, w, r, id)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func listIncidents(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	severity := q.Get("severity")
	assignedTo := q.Get("assigned_to")
	tenantID := q.Get("tenant_id")
	search := q.Get("search")

	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if status != "" {
		where += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	if severity != "" {
		where += fmt.Sprintf(" AND severity = $%d", argIdx)
		args = append(args, severity)
		argIdx++
	}
	if assignedTo != "" {
		where += fmt.Sprintf(" AND assigned_to = $%d", argIdx)
		args = append(args, assignedTo)
		argIdx++
	}
	if tenantID != "" {
		where += fmt.Sprintf(" AND tenant_id = $%d", argIdx)
		args = append(args, tenantID)
		argIdx++
	}
	if search != "" {
		where += fmt.Sprintf(" AND (title ILIKE $%d OR description ILIKE $%d)", argIdx, argIdx+1)
		searchPattern := "%" + search + "%"
		args = append(args, searchPattern, searchPattern)
		argIdx += 2
	}

	var incidents []Incident
	if err := db.Select(&incidents, "SELECT id, tenant_id, title, description, severity, status, assigned_to, timeline, created_at, updated_at FROM incidents "+where+" ORDER BY created_at DESC", args...); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list incidents")
		return
	}
	if incidents == nil {
		incidents = []Incident{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"incidents": incidents})
}

func createIncident(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID    string          `json:"tenant_id"`
		Title       string          `json:"title"`
		Description string          `json:"description"`
		Severity    string          `json:"severity"`
		AssignedTo  string          `json:"assigned_to"`
		Timeline    json.RawMessage `json:"timeline"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	validSeverities := map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
	if !validSeverities[req.Severity] {
		req.Severity = "medium"
	}

	if req.Timeline == nil {
		entry := []TimelineEntry{
			{Timestamp: time.Now().UTC(), Actor: "api", Action: "created", Detail: "Incident created"},
		}
		req.Timeline, _ = json.Marshal(entry)
	}

	id := uuid.New().String()
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO incidents (id, tenant_id, title, description, severity, status, assigned_to, timeline, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, 'open', $6, $7, $8, $8)`,
		id, req.TenantID, req.Title, req.Description, req.Severity, req.AssignedTo, req.Timeline, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create incident")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id, "status": "created"})
}

func getIncident(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	var inc Incident
	if err := db.Get(&inc, "SELECT id, tenant_id, title, description, severity, status, assigned_to, timeline, created_at, updated_at FROM incidents WHERE id = $1", id); err != nil {
		writeError(w, http.StatusNotFound, "incident not found")
		return
	}

	var relatedEvents []string
	if err := db.Select(&relatedEvents, "SELECT event_id FROM incident_events WHERE incident_id = $1", id); err == nil {
		_ = relatedEvents
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"incident": inc})
}

func updateIncident(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Title       *string `json:"title,omitempty"`
		Description *string `json:"description,omitempty"`
		Severity    *string `json:"severity,omitempty"`
		Status      *string `json:"status,omitempty"`
		AssignedTo  *string `json:"assigned_to,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1
	actor := r.Header.Get("X-Username")
	if actor == "" {
		actor = "api"
	}

	if req.Title != nil {
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, *req.Title)
		argIdx++
	}
	if req.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if req.Severity != nil {
		valid := map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
		if !valid[*req.Severity] {
			writeError(w, http.StatusBadRequest, "invalid severity")
			return
		}
		setClauses = append(setClauses, fmt.Sprintf("severity = $%d", argIdx))
		args = append(args, *req.Severity)
		(&IncidentManager{db: db}).addTimelineEntry(id, actor, "severity_changed", fmt.Sprintf("Severity changed to %s", *req.Severity))
		argIdx++
	}
	if req.Status != nil {
		valid := map[string]bool{"open": true, "investigating": true, "resolved": true, "closed": true}
		if !valid[*req.Status] {
			writeError(w, http.StatusBadRequest, "invalid status")
			return
		}
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *req.Status)
		(&IncidentManager{db: db}).addTimelineEntry(id, actor, "status_changed", fmt.Sprintf("Status changed to %s", *req.Status))
		argIdx++
	}
	if req.AssignedTo != nil {
		setClauses = append(setClauses, fmt.Sprintf("assigned_to = $%d", argIdx))
		args = append(args, *req.AssignedTo)
		(&IncidentManager{db: db}).addTimelineEntry(id, actor, "assigned", fmt.Sprintf("Assigned to %s", *req.AssignedTo))
		argIdx++
	}

	if len(setClauses) == 0 {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	args = append(args, id)
	query := fmt.Sprintf("UPDATE incidents SET %s WHERE id = $%d", strings.Join(setClauses, ", "), argIdx)
	result, err := db.Exec(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update incident")
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "incident not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func deleteIncident(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	result, err := db.Exec("DELETE FROM incidents WHERE id = $1", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete incident")
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "incident not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func addIncidentNote(db *sqlx.DB, w http.ResponseWriter, r *http.Request, incidentID string) {
	var req struct {
		Author string `json:"author"`
		Body   string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Body == "" {
		writeError(w, http.StatusBadRequest, "body is required")
		return
	}

	id := uuid.New().String()
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO incident_notes (id, incident_id, author, body, created_at) VALUES ($1, $2, $3, $4, $5)`,
		id, incidentID, req.Author, req.Body, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add note")
		return
	}

	(&IncidentManager{db: db}).addTimelineEntry(incidentID, req.Author, "note_added", "Note added to incident")
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id, "status": "created"})
}

func listIncidentNotes(db *sqlx.DB, w http.ResponseWriter, r *http.Request, incidentID string) {
	var notes []IncidentNote
	if err := db.Select(&notes, "SELECT id, incident_id, author, body, created_at FROM incident_notes WHERE incident_id = $1 ORDER BY created_at ASC", incidentID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list notes")
		return
	}
	if notes == nil {
		notes = []IncidentNote{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"notes": notes})
}

func escalateIncident(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Actor string `json:"actor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Actor = "system"
	}
	if req.Actor == "" {
		req.Actor = "system"
	}

	var inc Incident
	if err := db.Get(&inc, "SELECT id, severity FROM incidents WHERE id = $1", id); err != nil {
		writeError(w, http.StatusNotFound, "incident not found")
		return
	}

	newSeverity := "high"
	if inc.Severity == "low" {
		newSeverity = "medium"
	} else if inc.Severity == "medium" {
		newSeverity = "high"
	} else if inc.Severity == "high" {
		newSeverity = "critical"
	} else {
		writeError(w, http.StatusBadRequest, "incident already at critical severity")
		return
	}

	if _, err := db.Exec("UPDATE incidents SET severity = $1, status = 'investigating', updated_at = NOW() WHERE id = $2", newSeverity, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to escalate incident")
		return
	}

	(&IncidentManager{db: db}).addTimelineEntry(id, req.Actor, "escalated", fmt.Sprintf("Severity escalated to %s", newSeverity))
	writeJSON(w, http.StatusOK, map[string]string{"status": "escalated", "severity": newSeverity})
}
