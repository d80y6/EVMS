package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type LegalHold struct {
	ID         string     `json:"id" db:"id"`
	CameraID   string     `json:"camera_id" db:"camera_id"`
	Reason     string     `json:"reason" db:"reason"`
	CreatedBy  string     `json:"created_by" db:"created_by"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	ReleasedAt *time.Time `json:"released_at,omitempty" db:"released_at"`
}

type LegalHoldStore struct {
	db     *sqlx.DB
	mu     sync.RWMutex
	holds  map[string]bool
	logger *slog.Logger
}

func NewLegalHoldStore(db *sqlx.DB) *LegalHoldStore {
	return &LegalHoldStore{
		db:     db,
		holds:  make(map[string]bool),
		logger: slog.Default().With("component", "legal_hold"),
	}
}

func (s *LegalHoldStore) ImportLegacyHolds(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS legal_holds (
		id UUID PRIMARY KEY,
		camera_id TEXT NOT NULL,
		reason TEXT NOT NULL,
		created_by TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		released_at TIMESTAMPTZ
	)`)
	if err != nil {
		return fmt.Errorf("failed to create legal_holds table: %w", err)
	}
	return s.RefreshCache(ctx)
}

func (s *LegalHoldStore) RefreshCache(ctx context.Context) error {
	var holds []LegalHold
	err := s.db.SelectContext(ctx, &holds, "SELECT id, camera_id, reason, created_by, created_at, released_at FROM legal_holds WHERE released_at IS NULL")
	if err != nil {
		return fmt.Errorf("failed to refresh cache: %w", err)
	}
	s.mu.Lock()
	s.holds = make(map[string]bool, len(holds))
	for _, h := range holds {
		s.holds[h.CameraID] = true
	}
	s.mu.Unlock()
	s.logger.Info("Refreshed legal hold cache", "active_holds", len(holds))
	return nil
}

func (s *LegalHoldStore) IsOnHold(cameraID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.holds[cameraID]
}

func (s *LegalHoldStore) CreateHold(ctx context.Context, cameraID, reason, createdBy string) (*LegalHold, error) {
	hold := &LegalHold{
		ID:        uuid.New().String(),
		CameraID:  cameraID,
		Reason:    reason,
		CreatedBy: createdBy,
		CreatedAt: time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO legal_holds (id, camera_id, reason, created_by, created_at) VALUES ($1, $2, $3, $4, $5)",
		hold.ID, hold.CameraID, hold.Reason, hold.CreatedBy, hold.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create legal hold: %w", err)
	}
	s.mu.Lock()
	s.holds[hold.CameraID] = true
	s.mu.Unlock()
	s.logger.Info("Created legal hold", "id", hold.ID, "camera_id", hold.CameraID)
	return hold, nil
}

func (s *LegalHoldStore) ListHolds(ctx context.Context) ([]LegalHold, error) {
	var holds []LegalHold
	err := s.db.SelectContext(ctx, &holds, "SELECT id, camera_id, reason, created_by, created_at, released_at FROM legal_holds ORDER BY created_at DESC")
	if err != nil {
		return nil, fmt.Errorf("failed to list legal holds: %w", err)
	}
	return holds, nil
}

func (s *LegalHoldStore) GetHold(ctx context.Context, id string) (*LegalHold, error) {
	var hold LegalHold
	err := s.db.GetContext(ctx, &hold, "SELECT id, camera_id, reason, created_by, created_at, released_at FROM legal_holds WHERE id = $1", id)
	if err != nil {
		return nil, fmt.Errorf("legal hold not found: %w", err)
	}
	return &hold, nil
}

func (s *LegalHoldStore) ReleaseHold(ctx context.Context, id string) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, "UPDATE legal_holds SET released_at = $1 WHERE id = $2 AND released_at IS NULL", now, id)
	if err != nil {
		return fmt.Errorf("failed to release hold: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("legal hold not found or already released")
	}
	return s.RefreshCache(ctx)
}

// HTTP handlers

func handleCreateLegalHold(store *LegalHoldStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CameraID  string `json:"camera_id"`
			Reason    string `json:"reason"`
			CreatedBy string `json:"created_by"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.CameraID == "" || req.Reason == "" {
			jsonError(w, "camera_id and reason are required", http.StatusBadRequest)
			return
		}
		if req.CreatedBy == "" {
			req.CreatedBy = "admin"
		}

		hold, err := store.CreateHold(r.Context(), req.CameraID, req.Reason, req.CreatedBy)
		if err != nil {
			logger.Error("Failed to create legal hold", "error", err)
			jsonError(w, "failed to create legal hold", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(hold)
	}
}

func handleListLegalHolds(store *LegalHoldStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		holds, err := store.ListHolds(r.Context())
		if err != nil {
			logger.Error("Failed to list legal holds", "error", err)
			jsonError(w, "failed to list legal holds", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"legal_holds": holds})
	}
}

func handleReleaseLegalHold(store *LegalHoldStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := extractParam(r.URL.Path, "/legal-holds/")
		id = trimSuffix(id, "/release")
		if id == "" {
			jsonError(w, "id is required", http.StatusBadRequest)
			return
		}

		if err := store.ReleaseHold(r.Context(), id); err != nil {
			logger.Error("Failed to release legal hold", "id", id, "error", err)
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "released"})
	}
}

func extractParam(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	trimmed := strings.TrimPrefix(path, prefix)
	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		return trimmed[:idx]
	}
	return trimmed
}

func trimSuffix(s, suffix string) string {
	return strings.TrimSuffix(s, suffix)
}
