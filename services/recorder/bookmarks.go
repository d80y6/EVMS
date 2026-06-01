package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
)

type Bookmark struct {
	ID        string    `json:"id" db:"id"`
	CameraID  string    `json:"camera_id" db:"camera_id"`
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
	Label     string    `json:"label" db:"label"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	CreatedBy string    `json:"created_by" db:"created_by"`
}

func handleCreateBookmark(db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CameraID  string `json:"camera_id"`
			Timestamp string `json:"timestamp"`
			Label     string `json:"label"`
			CreatedBy string `json:"created_by"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request", http.StatusBadRequest)
			return
		}

		var id string
		err := db.QueryRowContext(r.Context(),
			"INSERT INTO bookmarks (camera_id, timestamp, label, created_by) VALUES ($1, $2, $3, $4) RETURNING id",
			req.CameraID, req.Timestamp, req.Label, req.CreatedBy).Scan(&id)
		if err != nil {
			jsonError(w, "failed to create bookmark", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "created"})
	}
}

func handleListBookmarks(db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cameraID := r.URL.Query().Get("camera_id")
		var bookmarks []Bookmark
		var err error
		if cameraID != "" {
			err = db.SelectContext(r.Context(), &bookmarks,
				"SELECT id, camera_id, timestamp, label, created_at, created_by FROM bookmarks WHERE camera_id = $1 ORDER BY timestamp DESC",
				cameraID)
		} else {
			err = db.SelectContext(r.Context(), &bookmarks,
				"SELECT id, camera_id, timestamp, label, created_at, created_by FROM bookmarks ORDER BY timestamp DESC LIMIT 100")
		}
		if err != nil {
			jsonError(w, "failed to list bookmarks", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"bookmarks": bookmarks})
	}
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
