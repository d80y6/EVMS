package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func cameraControlURL() string {
	if u := os.Getenv("CAMERA_CONTROL_URL"); u != "" {
		return u
	}
	return "http://camera-control:8088"
}

type TourStep struct {
	CameraID     string `json:"camera_id"`
	PresetToken  string `json:"preset_token,omitempty"`
	DwellSeconds int    `json:"dwell_seconds"`
}

type Tour struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Enabled   bool       `json:"enabled"`
	Steps     []TourStep `json:"steps"`
	Interval  int        `json:"interval"`
	CreatedAt time.Time  `json:"created_at"`
}

type TourScheduler struct {
	mu     sync.RWMutex
	tours  map[string]*Tour
	cancel map[string]context.CancelFunc
	db     *sqlx.DB
	logger *slog.Logger
}

func NewTourScheduler(db *sqlx.DB, logger *slog.Logger) *TourScheduler {
	ts := &TourScheduler{
		tours:  make(map[string]*Tour),
		cancel: make(map[string]context.CancelFunc),
		db:     db,
		logger: logger,
	}
	ts.loadFromDB()
	return ts
}

func (ts *TourScheduler) loadFromDB() {
	if ts.db == nil {
		return
	}
	var rows []struct {
		ID         string          `db:"id"`
		Name       string          `db:"name"`
		Enabled    bool            `db:"enabled"`
		Interval   int             `db:"interval_sec"`
		Steps      json.RawMessage `db:"steps"`
		CreatedAt  time.Time       `db:"created_at"`
	}
	if err := ts.db.Select(&rows, "SELECT id, name, enabled, interval_sec, steps, created_at FROM tours"); err != nil {
		ts.logger.Warn("Failed to load tours from DB", "error", err)
		return
	}
	for _, row := range rows {
		var steps []TourStep
		json.Unmarshal(row.Steps, &steps)
		ts.tours[row.ID] = &Tour{
			ID:        row.ID,
			Name:      row.Name,
			Enabled:   row.Enabled,
			Interval:  row.Interval,
			Steps:     steps,
			CreatedAt: row.CreatedAt,
		}
	}
	ts.logger.Info("Loaded tours from database", "count", len(rows))
}

func (ts *TourScheduler) saveTour(tour *Tour) error {
	if ts.db == nil {
		return nil
	}
	steps, _ := json.Marshal(tour.Steps)
	_, err := ts.db.Exec(`INSERT INTO tours (id, name, enabled, interval_sec, steps, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (id) DO UPDATE SET name=$2, enabled=$3, interval_sec=$4, steps=$5, updated_at=NOW()`,
		tour.ID, tour.Name, tour.Enabled, tour.Interval, steps, tour.CreatedAt)
	return err
}

func (ts *TourScheduler) deleteTour(id string) error {
	if ts.db == nil {
		return nil
	}
	_, err := ts.db.Exec("DELETE FROM tours WHERE id=$1", id)
	return err
}

func (ts *TourScheduler) AddTour(tour *Tour) {
	if tour.ID == "" {
		tour.ID = uuid.New().String()
	}
	tour.CreatedAt = time.Now().UTC()
	ts.mu.Lock()
	ts.tours[tour.ID] = tour
	ts.mu.Unlock()
	if err := ts.saveTour(tour); err != nil {
		ts.logger.Error("Failed to persist tour", "id", tour.ID, "error", err)
	}
}

func (ts *TourScheduler) RemoveTour(id string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if cancel, ok := ts.cancel[id]; ok {
		cancel()
	}
	delete(ts.tours, id)
	delete(ts.cancel, id)
	if err := ts.deleteTour(id); err != nil {
		ts.logger.Error("Failed to remove tour from DB", "id", id, "error", err)
	}
}

func (ts *TourScheduler) StartTour(id string) error {
	ts.mu.RLock()
	tour, ok := ts.tours[id]
	ts.mu.RUnlock()
	if !ok {
		return fmt.Errorf("tour not found: %s", id)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ts.mu.Lock()
	if oldCancel, ok := ts.cancel[id]; ok {
		oldCancel()
	}
	ts.cancel[id] = cancel
	ts.mu.Unlock()

	go ts.runTour(ctx, tour)
	return nil
}

func (ts *TourScheduler) StopTour(id string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if cancel, ok := ts.cancel[id]; ok {
		cancel()
		delete(ts.cancel, id)
	}
}

func (ts *TourScheduler) runTour(ctx context.Context, tour *Tour) {
	ticker := time.NewTicker(time.Duration(tour.Interval) * time.Second)
	defer ticker.Stop()

	stepIdx := 0
	for {
		select {
		case <-ticker.C:
			if len(tour.Steps) == 0 {
				continue
			}
			step := tour.Steps[stepIdx]
			ts.logger.Info("tour step", "tour", tour.Name, "camera", step.CameraID,
				"preset", step.PresetToken, "dwell", step.DwellSeconds)

			if step.PresetToken != "" {
				url := fmt.Sprintf("%s/cameras/%s/ptz/presets/%s/goto",
					cameraControlURL(), step.CameraID, step.PresetToken)
				client := &http.Client{Timeout: 5 * time.Second}
				resp, err := client.Post(url, "application/json", nil)
				if err != nil {
					ts.logger.Warn("tour PTZ goto failed", "tour", tour.Name, "camera", step.CameraID, "error", err)
				} else {
					resp.Body.Close()
				}
			}

			stepIdx = (stepIdx + 1) % len(tour.Steps)

		case <-ctx.Done():
			return
		}
	}
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (ts *TourScheduler) HandleHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/tours")

	switch {
	case r.Method == http.MethodGet && path == "":
		ts.mu.RLock()
		tours := make([]*Tour, 0, len(ts.tours))
		for _, t := range ts.tours {
			tours = append(tours, t)
		}
		ts.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"tours": tours})

	case r.Method == http.MethodPost && path == "":
		var tour Tour
		if err := json.NewDecoder(r.Body).Decode(&tour); err != nil {
			jsonError(w, "invalid tour", http.StatusBadRequest)
			return
		}
		ts.AddTour(&tour)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "created", "id": tour.ID})

	case r.Method == http.MethodPost && path == "/start":
		id := r.URL.Query().Get("id")
		if err := ts.StartTour(id); err != nil {
			jsonError(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "started"})

	case r.Method == http.MethodPost && path == "/stop":
		id := r.URL.Query().Get("id")
		ts.StopTour(id)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})

	case r.Method == http.MethodDelete && path == "":
		id := r.URL.Query().Get("id")
		ts.RemoveTour(id)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})

	default:
		jsonError(w, "not found", http.StatusNotFound)
	}
}
