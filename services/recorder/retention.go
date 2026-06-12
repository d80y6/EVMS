package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
)

type PerCameraRetentionPolicy struct {
	ID                  string    `json:"id" db:"id"`
	CameraID            string    `json:"camera_id" db:"camera_id"`
	RetentionDays       int       `json:"retention_days" db:"retention_days"`
	ArchiveEnabled      bool      `json:"archive_enabled" db:"archive_enabled"`
	ArchiveStorageClass string    `json:"archive_storage_class" db:"archive_storage_class"`
	MotionRetentionDays int       `json:"motion_retention_days" db:"motion_retention_days"`
	CreatedAt           time.Time `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time `json:"updated_at" db:"updated_at"`
}

type RetentionPolicyManager struct {
	db     *sqlx.DB
	cache  map[string]*PerCameraRetentionPolicy
	mu     sync.RWMutex
	logger *Recorder
}

func NewRetentionPolicyManager(db *sqlx.DB, r *Recorder) *RetentionPolicyManager {
	return &RetentionPolicyManager{
		db:     db,
		cache:  make(map[string]*PerCameraRetentionPolicy),
		logger: r,
	}
}

func (m *RetentionPolicyManager) RefreshCache(ctx context.Context) error {
	var policies []PerCameraRetentionPolicy
	err := m.db.SelectContext(ctx, &policies, "SELECT id, camera_id, retention_days, archive_enabled, archive_storage_class, motion_retention_days, created_at, updated_at FROM retention_policies")
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.cache = make(map[string]*PerCameraRetentionPolicy, len(policies))
	for _, p := range policies {
		cp := p
		m.cache[p.CameraID] = &cp
	}
	m.mu.Unlock()
	return nil
}

func (m *RetentionPolicyManager) GetEffectiveRetention(cameraID string) int {
	m.mu.RLock()
	p, ok := m.cache[cameraID]
	m.mu.RUnlock()
	if ok && p.RetentionDays > 0 {
		return p.RetentionDays
	}
	return m.logger.config.RetentionDays
}

func (m *RetentionPolicyManager) GetEffectiveMotionRetention(cameraID string) int {
	m.mu.RLock()
	p, ok := m.cache[cameraID]
	m.mu.RUnlock()
	if ok && p.MotionRetentionDays > 0 {
		return p.MotionRetentionDays
	}
	return m.logger.config.RetentionDays
}

func (m *RetentionPolicyManager) IsArchiveEnabled(cameraID string) bool {
	m.mu.RLock()
	p, ok := m.cache[cameraID]
	m.mu.RUnlock()
	return ok && p.ArchiveEnabled
}

func (m *RetentionPolicyManager) GetPolicy(ctx context.Context, cameraID string) (*PerCameraRetentionPolicy, error) {
	var p PerCameraRetentionPolicy
	err := m.db.GetContext(ctx, &p, "SELECT id, camera_id, retention_days, archive_enabled, archive_storage_class, motion_retention_days, created_at, updated_at FROM retention_policies WHERE camera_id = $1", cameraID)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (m *RetentionPolicyManager) UpsertPolicy(ctx context.Context, p *PerCameraRetentionPolicy) error {
	_, err := m.db.ExecContext(ctx, `INSERT INTO retention_policies (camera_id, retention_days, archive_enabled, archive_storage_class, motion_retention_days, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (camera_id) DO UPDATE SET
			retention_days = EXCLUDED.retention_days,
			archive_enabled = EXCLUDED.archive_enabled,
			archive_storage_class = EXCLUDED.archive_storage_class,
			motion_retention_days = EXCLUDED.motion_retention_days,
			updated_at = NOW()`,
		p.CameraID, p.RetentionDays, p.ArchiveEnabled, p.ArchiveStorageClass, p.MotionRetentionDays)
	if err != nil {
		return err
	}
	return m.RefreshCache(ctx)
}

func (m *RetentionPolicyManager) DeletePolicy(ctx context.Context, cameraID string) error {
	_, err := m.db.ExecContext(ctx, "DELETE FROM retention_policies WHERE camera_id = $1", cameraID)
	if err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.cache, cameraID)
	m.mu.Unlock()
	return nil
}

func (r *Recorder) runRetentionCleanupWithPolicies(ctx context.Context) {
	cutoff := time.Now().AddDate(0, 0, -r.config.RetentionDays)
	r.logger.Info("Running retention cleanup", "cutoff", cutoff)

	var segments []RecordingSegment
	err := r.db.SelectContext(ctx, &segments,
		"SELECT camera_id, file_path, start_time FROM recordings WHERE start_time < $1", cutoff)
	if err != nil {
		r.logger.Error("Failed to fetch expired segments", "error", err)
		return
	}

	deletedCount := 0
	policyCutoffs := make(map[string]time.Time)

	for _, seg := range segments {
		if r.legalHolds.IsOnHold(seg.CameraID) {
			continue
		}

		effectiveRetention, ok := policyCutoffs[seg.CameraID]
		if !ok {
			retentionDays := r.config.RetentionDays
			if r.policyManager != nil {
				retentionDays = r.policyManager.GetEffectiveRetention(seg.CameraID)
			}
			effectiveRetention = time.Now().AddDate(0, 0, -retentionDays)
			policyCutoffs[seg.CameraID] = effectiveRetention
		}

		if seg.StartTime.After(effectiveRetention) {
			continue
		}

		if err := os.Remove(seg.FilePath); err != nil && !os.IsNotExist(err) {
			r.logger.Error("Failed to delete recording file", "path", seg.FilePath, "error", err)
			continue
		}
		if err := os.Remove(seg.FilePath + ".preroll"); err != nil && !os.IsNotExist(err) {
			r.logger.Error("Failed to delete preroll file", "path", seg.FilePath+".preroll", "error", err)
		}

		if _, err := r.db.ExecContext(ctx,
			"DELETE FROM recordings WHERE file_path = $1", seg.FilePath); err != nil {
			r.logger.Error("Failed to delete recording record", "path", seg.FilePath, "error", err)
			continue
		}
		deletedCount++
	}

	r.logger.Info("Retention cleanup finished", "deleted_count", deletedCount)
}

func handleGetRetentionPolicy(pm *RetentionPolicyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cameraID := r.URL.Query().Get("camera_id")
		if cameraID == "" {
			jsonError(w, "camera_id is required", http.StatusBadRequest)
			return
		}
		p, err := pm.GetPolicy(r.Context(), cameraID)
		if err != nil {
			jsonError(w, "policy not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	}
}

func handleListRetentionPolicies(pm *RetentionPolicyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		policies := make([]PerCameraRetentionPolicy, 0)
		err := pm.db.SelectContext(r.Context(), &policies,
			"SELECT id, camera_id, retention_days, archive_enabled, archive_storage_class, motion_retention_days, created_at, updated_at FROM retention_policies ORDER BY camera_id")
		if err != nil {
			jsonError(w, "failed to list policies", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"policies":                policies,
			"global_retention_days":    pm.logger.config.RetentionDays,
			"global_archive_enabled":  false,
			"global_archive_after_days": 90,
		})
	}
}

func handleBulkRetentionUpdate(pm *RetentionPolicyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Policies []struct {
				CameraID      string `json:"camera_id"`
				RetentionDays int    `json:"retention_days"`
			} `json:"policies"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		count := 0
		for _, p := range req.Policies {
			if p.CameraID == "" || p.RetentionDays <= 0 {
				continue
			}
			if err := pm.UpsertPolicy(r.Context(), &PerCameraRetentionPolicy{
				CameraID:      p.CameraID,
				RetentionDays: p.RetentionDays,
			}); err != nil {
				pm.logger.logger.Error("Failed to bulk update retention", "camera_id", p.CameraID, "error", err)
				continue
			}
			count++
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "count": count})
	}
}

func handleCreateRetentionPolicy(pm *RetentionPolicyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CameraID            string `json:"camera_id"`
			RetentionDays       int    `json:"retention_days"`
			ArchiveEnabled      bool   `json:"archive_enabled"`
			ArchiveStorageClass string `json:"archive_storage_class"`
			MotionRetentionDays int    `json:"motion_retention_days"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.CameraID == "" {
			jsonError(w, "camera_id is required", http.StatusBadRequest)
			return
		}
		if req.RetentionDays <= 0 {
			req.RetentionDays = 7
		}
		if req.ArchiveStorageClass == "" {
			req.ArchiveStorageClass = "WARM"
		}
		if req.MotionRetentionDays <= 0 {
			req.MotionRetentionDays = req.RetentionDays
		}

		p := &PerCameraRetentionPolicy{
			CameraID:            req.CameraID,
			RetentionDays:       req.RetentionDays,
			ArchiveEnabled:      req.ArchiveEnabled,
			ArchiveStorageClass: req.ArchiveStorageClass,
			MotionRetentionDays: req.MotionRetentionDays,
		}

		if err := pm.UpsertPolicy(r.Context(), p); err != nil {
			jsonError(w, "failed to create policy", http.StatusInternalServerError)
			return
		}

		created, err := pm.GetPolicy(r.Context(), req.CameraID)
		if err != nil {
			jsonError(w, "policy created but failed to retrieve", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(created)
	}
}

func handleUpdateRetentionPolicy(pm *RetentionPolicyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cameraID := extractParam(r.URL.Path, "/retention-policies/")
		if cameraID == "" {
			jsonError(w, "camera_id in path required", http.StatusBadRequest)
			return
		}

		var req struct {
			RetentionDays       *int   `json:"retention_days"`
			ArchiveEnabled      *bool  `json:"archive_enabled"`
			ArchiveStorageClass string `json:"archive_storage_class"`
			MotionRetentionDays *int   `json:"motion_retention_days"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		existing, err := pm.GetPolicy(r.Context(), cameraID)
		if err != nil {
			jsonError(w, "policy not found", http.StatusNotFound)
			return
		}

		if req.RetentionDays != nil {
			existing.RetentionDays = *req.RetentionDays
		}
		if req.ArchiveEnabled != nil {
			existing.ArchiveEnabled = *req.ArchiveEnabled
		}
		if req.ArchiveStorageClass != "" {
			existing.ArchiveStorageClass = req.ArchiveStorageClass
		}
		if req.MotionRetentionDays != nil {
			existing.MotionRetentionDays = *req.MotionRetentionDays
		}

		if err := pm.UpsertPolicy(r.Context(), existing); err != nil {
			jsonError(w, "failed to update policy", http.StatusInternalServerError)
			return
		}

		updated, _ := pm.GetPolicy(r.Context(), cameraID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(updated)
	}
}

func handleDeleteRetentionPolicy(pm *RetentionPolicyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cameraID := extractParam(r.URL.Path, "/retention-policies/")
		if cameraID == "" {
			jsonError(w, "camera_id in path required", http.StatusBadRequest)
			return
		}
		if err := pm.DeletePolicy(r.Context(), cameraID); err != nil {
			jsonError(w, "failed to delete policy", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	}
}

func handleUpdateGlobalRetention(pm *RetentionPolicyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RetentionDays    *int  `json:"retention_days"`
			ArchiveEnabled   *bool `json:"archive_enabled"`
			ArchiveAfterDays *int  `json:"archive_after_days"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.RetentionDays != nil && *req.RetentionDays > 0 {
			pm.logger.config.RetentionDays = *req.RetentionDays
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

func (r *Recorder) StartRetentionPolicyWorker(ctx context.Context) {
	ticker := time.NewTicker(r.config.CleanupInterval)
	defer ticker.Stop()

	r.logger.Info("Starting retention policy worker", "interval", r.config.CleanupInterval)

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("Stopping retention policy worker")
			return
		case <-ticker.C:
			if err := r.policyManager.RefreshCache(ctx); err != nil {
				r.logger.Error("Failed to refresh retention policy cache", "error", err)
			}
		}
	}
}
