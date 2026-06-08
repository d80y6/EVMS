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
)

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
	logger *slog.Logger
}

func NewTourScheduler(logger *slog.Logger) *TourScheduler {
	return &TourScheduler{
		tours:  make(map[string]*Tour),
		cancel: make(map[string]context.CancelFunc),
		logger: logger,
	}
}

func (ts *TourScheduler) AddTour(tour *Tour) {
	if tour.ID == "" {
		tour.ID = uuid.New().String()
	}
	tour.CreatedAt = time.Now().UTC()
	ts.mu.Lock()
	ts.tours[tour.ID] = tour
	ts.mu.Unlock()
}

func (ts *TourScheduler) RemoveTour(id string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if cancel, ok := ts.cancel[id]; ok {
		cancel()
	}
	delete(ts.tours, id)
	delete(ts.cancel, id)
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
				http.Post(fmt.Sprintf("http://camera-control:8088/cameras/%s/ptz/presets/%s/goto",
					step.CameraID, step.PresetToken), "application/json", nil)
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
