package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type EvidenceCase struct {
	ID         string    `db:"id" json:"id"`
	TenantID   string    `db:"tenant_id" json:"tenant_id"`
	Name       string    `db:"name" json:"name"`
	CaseNumber string    `db:"case_number" json:"case_number"`
	Status     string    `db:"status" json:"status"`
	AssignedTo string    `db:"assigned_to" json:"assigned_to"`
	Tags       string    `db:"tags" json:"tags"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time `db:"updated_at" json:"updated_at"`
}

type EvidenceLocker struct {
	ID          string    `db:"id" json:"id"`
	CaseID      string    `db:"case_id" json:"case_id"`
	Name        string    `db:"name" json:"name"`
	Description string    `db:"description" json:"description"`
	CreatedBy   string    `db:"created_by" json:"created_by"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

type EvidenceItem struct {
	ID          string          `db:"id" json:"id"`
	LockerID    string          `db:"locker_id" json:"locker_id"`
	RecordingID string          `db:"recording_id" json:"recording_id"`
	CameraID    string          `db:"camera_id" json:"camera_id"`
	StartTime   *time.Time      `db:"start_time" json:"start_time,omitempty"`
	EndTime     *time.Time      `db:"end_time" json:"end_time,omitempty"`
	FilePath    string          `db:"file_path" json:"file_path"`
	SHA256      string          `db:"sha256" json:"sha256"`
	SizeBytes   int64           `db:"size_bytes" json:"size_bytes"`
	MimeType    string          `db:"mime_type" json:"mime_type"`
	Metadata    json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt   time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time       `db:"updated_at" json:"updated_at"`
}

type EvidenceShare struct {
	ID         string    `db:"id" json:"id"`
	ItemID     string    `db:"item_id" json:"item_id"`
	ShareToken string    `db:"share_token" json:"share_token"`
	ExpiresAt  time.Time `db:"expires_at" json:"expires_at"`
	CreatedBy  string    `db:"created_by" json:"created_by"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
}

type EvidenceAccessLog struct {
	ID         string    `db:"id" json:"id"`
	ItemID     string    `db:"item_id" json:"item_id"`
	Actor      string    `db:"actor" json:"actor"`
	Action     string    `db:"action" json:"action"`
	IPAddress  string    `db:"ip_address" json:"ip_address"`
	AccessedAt time.Time `db:"accessed_at" json:"accessed_at"`
}

type EvidenceManager struct {
	db     *sqlx.DB
	logger *slog.Logger
}

func NewEvidenceManager(db *sqlx.DB, logger *slog.Logger) *EvidenceManager {
	return &EvidenceManager{db: db, logger: logger}
}

func generateShareToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func computeSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), size, nil
}

func pqStringArray(s []string) interface{} {
	if s == nil {
		s = []string{}
	}
	return "{" + strings.Join(s, ",") + "}"
}

func handleEvidenceCases(db *sqlx.DB, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			jsonError(w, "database not configured", http.StatusInternalServerError)
			return
		}
		switch r.Method {
		case http.MethodGet:
			listEvidenceCases(db, w, r)
		case http.MethodPost:
			createEvidenceCase(db, w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleEvidenceCaseByID(db *sqlx.DB, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			jsonError(w, "database not configured", http.StatusInternalServerError)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/evidence/cases/")
		if id == "" {
			http.Error(w, "case id required", http.StatusBadRequest)
			return
		}
		if strings.Contains(id, "/") {
			id = strings.Split(id, "/")[0]
		}
		switch r.Method {
		case http.MethodGet:
			getEvidenceCase(db, w, r, id)
		case http.MethodPut:
			updateEvidenceCase(db, w, r, id)
		case http.MethodDelete:
			deleteEvidenceCase(db, w, r, id)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleEvidenceLockers(db *sqlx.DB, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			jsonError(w, "database not configured", http.StatusInternalServerError)
			return
		}
		switch r.Method {
		case http.MethodGet:
			listEvidenceLockers(db, w, r)
		case http.MethodPost:
			createEvidenceLocker(db, w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleEvidenceLockerByID(db *sqlx.DB, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			jsonError(w, "database not configured", http.StatusInternalServerError)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/evidence/lockers/")
		if id == "" {
			http.Error(w, "locker id required", http.StatusBadRequest)
			return
		}
		if strings.Contains(id, "/") {
			id = strings.Split(id, "/")[0]
		}
		switch r.Method {
		case http.MethodGet:
			getEvidenceLocker(db, w, r, id)
		case http.MethodPut:
			updateEvidenceLocker(db, w, r, id)
		case http.MethodDelete:
			deleteEvidenceLocker(db, w, r, id)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleEvidenceItems(db *sqlx.DB, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			jsonError(w, "database not configured", http.StatusInternalServerError)
			return
		}
		switch r.Method {
		case http.MethodGet:
			listEvidenceItems(db, w, r)
		case http.MethodPost:
			createEvidenceItem(db, w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleEvidenceItemByID(db *sqlx.DB, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			jsonError(w, "database not configured", http.StatusInternalServerError)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/evidence/items/")
		parts := strings.SplitN(path, "/", 2)
		id := parts[0]
		if id == "" {
			http.Error(w, "item id required", http.StatusBadRequest)
			return
		}

		if len(parts) == 2 && parts[1] == "share" {
			if r.Method == http.MethodPost {
				shareEvidenceItem(db, w, r, id)
				return
			}
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if len(parts) == 2 && parts[1] == "export" {
			if r.Method == http.MethodPost {
				exportEvidenceItem(db, w, r, id, logger)
				return
			}
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if len(parts) == 2 && parts[1] == "access-log" {
			if r.Method == http.MethodGet {
				getEvidenceAccessLog(db, w, r, id)
				return
			}
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		switch r.Method {
		case http.MethodGet:
			getEvidenceItem(db, w, r, id)
		case http.MethodPut:
			updateEvidenceItem(db, w, r, id)
		case http.MethodDelete:
			deleteEvidenceItem(db, w, r, id)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleShareAccess(db *sqlx.DB, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			jsonError(w, "database not configured", http.StatusInternalServerError)
			return
		}
		token := strings.TrimPrefix(r.URL.Path, "/api/evidence/share/")
		if token == "" {
			http.Error(w, "share token required", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var share EvidenceShare
		if err := db.Get(&share, "SELECT id, item_id, share_token, expires_at, created_by, created_at FROM evidence_shares WHERE share_token = $1", token); err != nil {
			jsonError(w, "share not found or expired", http.StatusNotFound)
			return
		}
		if time.Now().After(share.ExpiresAt) {
			jsonError(w, "share link has expired", http.StatusGone)
			return
		}

		var item EvidenceItem
		if err := db.Get(&item, "SELECT id, locker_id, recording_id, camera_id, start_time, end_time, file_path, sha256, size_bytes, mime_type, metadata, created_at, updated_at FROM evidence_items WHERE id = $1", share.ItemID); err != nil {
			jsonError(w, "evidence item not found", http.StatusNotFound)
			return
		}

		db.Exec(`INSERT INTO evidence_access_log (id, item_id, actor, action, ip_address, accessed_at) VALUES ($1, $2, $3, 'share_view', $4, NOW())`,
			uuid.New().String(), item.ID, "anonymous", r.RemoteAddr)

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"item":       item,
			"created_by": share.CreatedBy,
			"expires_at": share.ExpiresAt,
		})
	}
}

func listEvidenceCases(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	tenantID := q.Get("tenant_id")

	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1
	if status != "" {
		where += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	if tenantID != "" {
		where += fmt.Sprintf(" AND tenant_id = $%d", argIdx)
		args = append(args, tenantID)
		argIdx++
	}

	var cases []EvidenceCase
	if err := db.Select(&cases, "SELECT id, tenant_id, name, case_number, status, assigned_to, tags, created_at, updated_at FROM evidence_cases "+where+" ORDER BY created_at DESC", args...); err != nil {
		jsonError(w, "failed to list evidence cases", http.StatusInternalServerError)
		return
	}
	if cases == nil {
		cases = []EvidenceCase{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"cases": cases})
}

func createEvidenceCase(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID   string   `json:"tenant_id"`
		Name       string   `json:"name"`
		CaseNumber string   `json:"case_number"`
		AssignedTo string   `json:"assigned_to"`
		Tags       []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}
	id := uuid.New().String()
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO evidence_cases (id, tenant_id, name, case_number, status, assigned_to, tags, created_at, updated_at) VALUES ($1, $2, $3, $4, 'open', $5, $6, $7, $7)`,
		id, req.TenantID, req.Name, req.CaseNumber, req.AssignedTo, pqStringArray(req.Tags), now,
	); err != nil {
		jsonError(w, "failed to create evidence case", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id, "status": "created"})
}

func getEvidenceCase(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	var c EvidenceCase
	if err := db.Get(&c, "SELECT id, tenant_id, name, case_number, status, assigned_to, tags, created_at, updated_at FROM evidence_cases WHERE id = $1", id); err != nil {
		jsonError(w, "evidence case not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"case": c})
}

func updateEvidenceCase(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Name       *string  `json:"name,omitempty"`
		CaseNumber *string  `json:"case_number,omitempty"`
		Status     *string  `json:"status,omitempty"`
		AssignedTo *string  `json:"assigned_to,omitempty"`
		Tags       []string `json:"tags,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
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
	if req.CaseNumber != nil {
		setClauses = append(setClauses, fmt.Sprintf("case_number = $%d", argIdx))
		args = append(args, *req.CaseNumber)
		argIdx++
	}
	if req.Status != nil {
		valid := map[string]bool{"open": true, "closed": true, "archived": true}
		if !valid[*req.Status] {
			jsonError(w, "invalid status", http.StatusBadRequest)
			return
		}
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *req.Status)
		argIdx++
	}
	if req.AssignedTo != nil {
		setClauses = append(setClauses, fmt.Sprintf("assigned_to = $%d", argIdx))
		args = append(args, *req.AssignedTo)
		argIdx++
	}
	if req.Tags != nil {
		setClauses = append(setClauses, fmt.Sprintf("tags = $%d", argIdx))
		args = append(args, pqStringArray(req.Tags))
		argIdx++
	}
	if len(setClauses) == 0 {
		jsonError(w, "no fields to update", http.StatusBadRequest)
		return
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	args = append(args, id)
	query := fmt.Sprintf("UPDATE evidence_cases SET %s WHERE id = $%d", strings.Join(setClauses, ", "), argIdx)
	result, err := db.Exec(query, args...)
	if err != nil {
		jsonError(w, "failed to update case", http.StatusInternalServerError)
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		jsonError(w, "case not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func deleteEvidenceCase(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	result, err := db.Exec("DELETE FROM evidence_cases WHERE id = $1", id)
	if err != nil {
		jsonError(w, "failed to delete case", http.StatusInternalServerError)
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		jsonError(w, "case not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func listEvidenceLockers(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	caseID := r.URL.Query().Get("case_id")
	var lockers []EvidenceLocker
	var err error
	if caseID != "" {
		err = db.Select(&lockers, "SELECT id, case_id, name, description, created_by, created_at, updated_at FROM evidence_lockers WHERE case_id = $1 ORDER BY name", caseID)
	} else {
		err = db.Select(&lockers, "SELECT id, case_id, name, description, created_by, created_at, updated_at FROM evidence_lockers ORDER BY created_at DESC")
	}
	if err != nil {
		jsonError(w, "failed to list lockers", http.StatusInternalServerError)
		return
	}
	if lockers == nil {
		lockers = []EvidenceLocker{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"lockers": lockers})
}

func createEvidenceLocker(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	var req struct {
		CaseID      string `json:"case_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		CreatedBy   string `json:"created_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.CaseID == "" {
		jsonError(w, "case_id and name are required", http.StatusBadRequest)
		return
	}
	id := uuid.New().String()
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO evidence_lockers (id, case_id, name, description, created_by, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		id, req.CaseID, req.Name, req.Description, req.CreatedBy, now,
	); err != nil {
		jsonError(w, "failed to create locker", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id, "status": "created"})
}

func getEvidenceLocker(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	var l EvidenceLocker
	if err := db.Get(&l, "SELECT id, case_id, name, description, created_by, created_at, updated_at FROM evidence_lockers WHERE id = $1", id); err != nil {
		jsonError(w, "locker not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"locker": l})
}

func updateEvidenceLocker(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Name        *string `json:"name,omitempty"`
		Description *string `json:"description,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
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
	if req.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if len(setClauses) == 0 {
		jsonError(w, "no fields to update", http.StatusBadRequest)
		return
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	args = append(args, id)
	query := fmt.Sprintf("UPDATE evidence_lockers SET %s WHERE id = $%d", strings.Join(setClauses, ", "), argIdx)
	result, err := db.Exec(query, args...)
	if err != nil {
		jsonError(w, "failed to update locker", http.StatusInternalServerError)
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		jsonError(w, "locker not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func deleteEvidenceLocker(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	result, err := db.Exec("DELETE FROM evidence_lockers WHERE id = $1", id)
	if err != nil {
		jsonError(w, "failed to delete locker", http.StatusInternalServerError)
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		jsonError(w, "locker not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func listEvidenceItems(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	lockerID := r.URL.Query().Get("locker_id")
	cameraID := r.URL.Query().Get("camera_id")

	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1
	if lockerID != "" {
		where += fmt.Sprintf(" AND locker_id = $%d", argIdx)
		args = append(args, lockerID)
		argIdx++
	}
	if cameraID != "" {
		where += fmt.Sprintf(" AND camera_id = $%d", argIdx)
		args = append(args, cameraID)
		argIdx++
	}

	var items []EvidenceItem
	if err := db.Select(&items, "SELECT id, locker_id, recording_id, camera_id, start_time, end_time, file_path, sha256, size_bytes, mime_type, metadata, created_at, updated_at FROM evidence_items "+where+" ORDER BY created_at DESC", args...); err != nil {
		jsonError(w, "failed to list evidence items", http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = []EvidenceItem{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func createEvidenceItem(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	var req struct {
		LockerID    string          `json:"locker_id"`
		RecordingID string          `json:"recording_id"`
		CameraID    string          `json:"camera_id"`
		StartTime   *time.Time      `json:"start_time"`
		EndTime     *time.Time      `json:"end_time"`
		FilePath    string          `json:"file_path"`
		MimeType    string          `json:"mime_type"`
		Metadata    json.RawMessage `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.LockerID == "" || req.FilePath == "" {
		jsonError(w, "locker_id and file_path are required", http.StatusBadRequest)
		return
	}

	sha256sum, size, err := computeSHA256(req.FilePath)
	if err != nil {
		jsonError(w, "failed to compute file hash: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.MimeType == "" {
		req.MimeType = "video/mp4"
	}
	if req.Metadata == nil {
		req.Metadata = json.RawMessage("{}")
	}

	id := uuid.New().String()
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO evidence_items (id, locker_id, recording_id, camera_id, start_time, end_time, file_path, sha256, size_bytes, mime_type, metadata, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $12)`,
		id, req.LockerID, req.RecordingID, req.CameraID, req.StartTime, req.EndTime, req.FilePath, sha256sum, size, req.MimeType, req.Metadata, now,
	); err != nil {
		jsonError(w, "failed to create evidence item", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id, "sha256": sha256sum, "size_bytes": size, "status": "created"})
}

func getEvidenceItem(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	var item EvidenceItem
	if err := db.Get(&item, "SELECT id, locker_id, recording_id, camera_id, start_time, end_time, file_path, sha256, size_bytes, mime_type, metadata, created_at, updated_at FROM evidence_items WHERE id = $1", id); err != nil {
		jsonError(w, "evidence item not found", http.StatusNotFound)
		return
	}

	actor := r.Header.Get("X-Username")
	if actor == "" {
		actor = "api"
	}
	db.Exec(`INSERT INTO evidence_access_log (id, item_id, actor, action, ip_address, accessed_at) VALUES ($1, $2, $3, 'view', $4, NOW())`,
		uuid.New().String(), id, actor, r.RemoteAddr)

	writeJSON(w, http.StatusOK, map[string]interface{}{"item": item})
}

func updateEvidenceItem(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		RecordingID *string          `json:"recording_id,omitempty"`
		CameraID    *string          `json:"camera_id,omitempty"`
		StartTime   *time.Time       `json:"start_time,omitempty"`
		EndTime     *time.Time       `json:"end_time,omitempty"`
		MimeType    *string          `json:"mime_type,omitempty"`
		Metadata    *json.RawMessage `json:"metadata,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1
	if req.RecordingID != nil {
		setClauses = append(setClauses, fmt.Sprintf("recording_id = $%d", argIdx))
		args = append(args, *req.RecordingID)
		argIdx++
	}
	if req.CameraID != nil {
		setClauses = append(setClauses, fmt.Sprintf("camera_id = $%d", argIdx))
		args = append(args, *req.CameraID)
		argIdx++
	}
	if req.StartTime != nil {
		setClauses = append(setClauses, fmt.Sprintf("start_time = $%d", argIdx))
		args = append(args, *req.StartTime)
		argIdx++
	}
	if req.EndTime != nil {
		setClauses = append(setClauses, fmt.Sprintf("end_time = $%d", argIdx))
		args = append(args, *req.EndTime)
		argIdx++
	}
	if req.MimeType != nil {
		setClauses = append(setClauses, fmt.Sprintf("mime_type = $%d", argIdx))
		args = append(args, *req.MimeType)
		argIdx++
	}
	if req.Metadata != nil {
		setClauses = append(setClauses, fmt.Sprintf("metadata = $%d", argIdx))
		args = append(args, *req.Metadata)
		argIdx++
	}
	if len(setClauses) == 0 {
		jsonError(w, "no fields to update", http.StatusBadRequest)
		return
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	args = append(args, id)
	query := fmt.Sprintf("UPDATE evidence_items SET %s WHERE id = $%d", strings.Join(setClauses, ", "), argIdx)
	result, err := db.Exec(query, args...)
	if err != nil {
		jsonError(w, "failed to update evidence item", http.StatusInternalServerError)
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		jsonError(w, "evidence item not found", http.StatusNotFound)
		return
	}

	actor := r.Header.Get("X-Username")
	if actor == "" {
		actor = "api"
	}
	db.Exec(`INSERT INTO evidence_access_log (id, item_id, actor, action, ip_address, accessed_at) VALUES ($1, $2, $3, 'update', $4, NOW())`,
		uuid.New().String(), id, actor, r.RemoteAddr)

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func deleteEvidenceItem(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	result, err := db.Exec("DELETE FROM evidence_items WHERE id = $1", id)
	if err != nil {
		jsonError(w, "failed to delete evidence item", http.StatusInternalServerError)
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		jsonError(w, "evidence item not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func shareEvidenceItem(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		ExpiresInMinutes int    `json:"expires_in_minutes"`
		CreatedBy        string `json:"created_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.ExpiresInMinutes <= 0 {
		req.ExpiresInMinutes = 1440
	}

	var exists int
	if err := db.Get(&exists, "SELECT COUNT(*) FROM evidence_items WHERE id = $1", id); err != nil || exists == 0 {
		jsonError(w, "evidence item not found", http.StatusNotFound)
		return
	}

	token, err := generateShareToken()
	if err != nil {
		jsonError(w, "failed to generate share token", http.StatusInternalServerError)
		return
	}

	shareID := uuid.New().String()
	expiresAt := time.Now().UTC().Add(time.Duration(req.ExpiresInMinutes) * time.Minute)
	if _, err := db.Exec(
		`INSERT INTO evidence_shares (id, item_id, share_token, expires_at, created_by, created_at) VALUES ($1, $2, $3, $4, $5, NOW())`,
		shareID, id, token, expiresAt, req.CreatedBy,
	); err != nil {
		jsonError(w, "failed to create share link", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"share_token": token,
		"share_url":   "/api/evidence/share/" + token,
		"expires_at":  expiresAt,
	})
}

func exportEvidenceItem(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string, logger *slog.Logger) {
	var item EvidenceItem
	if err := db.Get(&item, "SELECT id, locker_id, recording_id, camera_id, start_time, end_time, file_path, sha256, size_bytes, mime_type, metadata, created_at, updated_at FROM evidence_items WHERE id = $1", id); err != nil {
		jsonError(w, "evidence item not found", http.StatusNotFound)
		return
	}

	manifest := map[string]interface{}{
		"evidence_id": item.ID,
		"camera_id":   item.CameraID,
		"recording_id": item.RecordingID,
		"start_time":  item.StartTime,
		"end_time":    item.EndTime,
		"sha256":      item.SHA256,
		"size_bytes":  item.SizeBytes,
		"mime_type":   item.MimeType,
		"metadata":    item.Metadata,
		"exported_at": time.Now().UTC(),
	}

	exportDir := "/exports/evidence"
	os.MkdirAll(exportDir, 0755)

	manifestPath := filepath.Join(exportDir, fmt.Sprintf("%s_manifest.json", id))
	f, err := os.Create(manifestPath)
	if err != nil {
		jsonError(w, "failed to create manifest", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(f).Encode(manifest)
	f.Close()

	actor := r.Header.Get("X-Username")
	if actor == "" {
		actor = "api"
	}
	db.Exec(`INSERT INTO evidence_access_log (id, item_id, actor, action, ip_address, accessed_at) VALUES ($1, $2, $3, 'export', $4, NOW())`,
		uuid.New().String(), id, actor, r.RemoteAddr)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"manifest": manifestPath,
		"file":     item.FilePath,
		"status":   "exported",
	})
}

func getEvidenceAccessLog(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	var logs []EvidenceAccessLog
	if err := db.Select(&logs, "SELECT id, item_id, actor, action, ip_address, accessed_at FROM evidence_access_log WHERE item_id = $1 ORDER BY accessed_at DESC", id); err != nil {
		jsonError(w, "failed to query access log", http.StatusInternalServerError)
		return
	}
	if logs == nil {
		logs = []EvidenceAccessLog{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"access_log": logs})
}
