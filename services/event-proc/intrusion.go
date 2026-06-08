package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type IntrusionZone struct {
	ID            string    `json:"id"`
	CameraID      string    `json:"camera_id"`
	Name          string    `json:"name"`
	PolygonPoints []Point   `json:"polygon_points"`
	Direction     string    `json:"direction"`
	Sensitivity   float64   `json:"sensitivity"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
}

type IntrusionEvent struct {
	ID         string    `json:"id"`
	ZoneID     string    `json:"zone_id"`
	CameraID   string    `json:"camera_id"`
	Direction  string    `json:"direction"`
	Confidence float64   `json:"confidence"`
	Timestamp  time.Time `json:"timestamp"`
	TrackID    string    `json:"track_id"`
}

type IntrusionZoneManager struct {
	mu    sync.RWMutex
	zones map[string]*IntrusionZone
}

func NewIntrusionZoneManager() *IntrusionZoneManager {
	return &IntrusionZoneManager{
		zones: make(map[string]*IntrusionZone),
	}
}

func (m *IntrusionZoneManager) List(cameraID string) []*IntrusionZone {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*IntrusionZone
	for _, z := range m.zones {
		if cameraID == "" || z.CameraID == cameraID {
			result = append(result, z)
		}
	}
	return result
}

func (m *IntrusionZoneManager) Get(id string) (*IntrusionZone, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	z, ok := m.zones[id]
	if !ok {
		return nil, fmt.Errorf("intrusion zone not found: %s", id)
	}
	return z, nil
}

func (m *IntrusionZoneManager) Create(zone *IntrusionZone) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	zone.ID = uuid.New().String()
	zone.CreatedAt = time.Now().UTC()
	if zone.Sensitivity == 0 {
		zone.Sensitivity = 0.8
	}
	if zone.Direction == "" {
		zone.Direction = "any"
	}
	m.zones[zone.ID] = zone
	return nil
}

func (m *IntrusionZoneManager) Update(zone *IntrusionZone) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.zones[zone.ID]
	if !ok {
		return fmt.Errorf("intrusion zone not found: %s", zone.ID)
	}
	zone.CreatedAt = existing.CreatedAt
	m.zones[zone.ID] = zone
	return nil
}

func (m *IntrusionZoneManager) Delete(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.zones[id]
	if !ok {
		return false
	}
	delete(m.zones, id)
	return true
}

func (m *IntrusionZoneManager) GetActive(cameraID string) []*IntrusionZone {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*IntrusionZone
	for _, z := range m.zones {
		if z.Enabled && (cameraID == "" || z.CameraID == cameraID) {
			result = append(result, z)
		}
	}
	return result
}

func centroid(bbox []float64) Point {
	if len(bbox) < 4 {
		return Point{}
	}
	return Point{
		X: (bbox[0] + bbox[2]) / 2,
		Y: (bbox[1] + bbox[3]) / 2,
	}
}

func polygonToFloat64(polygon []Point) [][2]float64 {
	result := make([][2]float64, len(polygon))
	for i, p := range polygon {
		result[i] = [2]float64{p.X, p.Y}
	}
	return result
}

func PointInPolygon(pt Point, polygon []Point) bool {
	n := len(polygon)
	inside := false
	j := n - 1
	for i := 0; i < n; i++ {
		if ((polygon[i].Y > pt.Y) != (polygon[j].Y > pt.Y)) &&
			(pt.X < (polygon[j].X-polygon[i].X)*(pt.Y-polygon[i].Y)/(polygon[j].Y-polygon[i].Y)+polygon[i].X) {
			inside = !inside
		}
		j = i
	}
	return inside
}

func calculateDirection(prev, curr Point) string {
	dx := curr.X - prev.X
	dy := curr.Y - prev.Y

	if math.Abs(dx) > math.Abs(dy) {
		if dx > 0 {
			return "right"
		}
		return "left"
	}
	if dy > 0 {
		return "down"
	}
	return "up"
}

type trackState struct {
	PrevCenter Point
	LastSeen   time.Time
	Label      string
	Confidence float64
}

type trackHistory struct {
	Centers []Point
	Updated time.Time
}

type IntrusionDetector struct {
	zoneManager *IntrusionZoneManager
	tracks      map[string]*trackHistory
	mu          sync.RWMutex
	logger      *slog.Logger
	db          *sqlx.DB
	stopCh      chan struct{}
}

func NewIntrusionDetector(db *sqlx.DB, logger *slog.Logger) *IntrusionDetector {
	return &IntrusionDetector{
		zoneManager: NewIntrusionZoneManager(),
		tracks:      make(map[string]*trackHistory),
		logger:      logger,
		db:          db,
		stopCh:      make(chan struct{}),
	}
}

func (d *IntrusionDetector) Start() {
	go d.cleanupLoop()
}

func (d *IntrusionDetector) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			d.mu.Lock()
			now := time.Now()
			for id, th := range d.tracks {
				if now.Sub(th.Updated) > 5*time.Second {
					delete(d.tracks, id)
				}
			}
			d.mu.Unlock()
		case <-d.stopCh:
			return
		}
	}
}

func (d *IntrusionDetector) Stop() {
	close(d.stopCh)
}

func (d *IntrusionDetector) Evaluate(cameraID string, tracks []Track) []IntrusionEvent {
	zones := d.zoneManager.GetActive(cameraID)
	if len(zones) == 0 {
		return nil
	}

	d.mu.Lock()
	now := time.Now()
	for _, tr := range tracks {
		if _, ok := d.tracks[tr.TrackID]; !ok {
			d.tracks[tr.TrackID] = &trackHistory{
				Centers: []Point{},
			}
		}
		th := d.tracks[tr.TrackID]
		c := centroid(tr.BBox)
		th.Centers = append(th.Centers, c)
		if len(th.Centers) > 10 {
			th.Centers = th.Centers[len(th.Centers)-10:]
		}
		th.Updated = now
	}
	d.mu.Unlock()

	var events []IntrusionEvent

	d.mu.RLock()
	for _, tr := range tracks {
		th, ok := d.tracks[tr.TrackID]
		if !ok || len(th.Centers) < 2 {
			continue
		}

		prev := th.Centers[len(th.Centers)-2]
		curr := th.Centers[len(th.Centers)-1]
		dir := calculateDirection(prev, curr)

		for _, zone := range zones {
			if !PointInPolygon(curr, zone.PolygonPoints) {
				continue
			}
			if zone.Direction != "any" && zone.Direction != dir {
				continue
			}

			event := IntrusionEvent{
				ID:         uuid.New().String(),
				ZoneID:     zone.ID,
				CameraID:   cameraID,
				Direction:  dir,
				Confidence: tr.Confidence,
				Timestamp:  now,
				TrackID:    tr.TrackID,
			}
			events = append(events, event)

			if d.db != nil {
				d.db.Exec(
					`INSERT INTO intrusion_events (id, zone_id, camera_id, direction, confidence, event_time, track_id)
					 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
					event.ID, event.ZoneID, event.CameraID, event.Direction, event.Confidence, event.Timestamp, event.TrackID)
			}
		}
	}
	d.mu.RUnlock()

	return events
}

func (d *IntrusionDetector) HandleHTTP(w http.ResponseWriter, r *http.Request) {
	id := ""
	path := r.URL.Path
	if strings.HasPrefix(path, "/api/intrusion-zones/") {
		id = path[len("/api/intrusion-zones/"):]
	}

	switch r.Method {
	case http.MethodGet:
		if id == "" {
			cameraID := r.URL.Query().Get("camera_id")
			zones := d.zoneManager.List(cameraID)
			if zones == nil {
				zones = []*IntrusionZone{}
			}
			writeJSON(w, http.StatusOK, zones)
		} else {
			zone, err := d.zoneManager.Get(id)
			if err != nil {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, zone)
		}

	case http.MethodPost:
		if id == "" {
			var zone IntrusionZone
			if err := json.NewDecoder(r.Body).Decode(&zone); err != nil {
				writeError(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			if err := d.zoneManager.Create(&zone); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, zone)
		} else {
			http.NotFound(w, r)
		}

	case http.MethodPut:
		if id != "" {
			var zone IntrusionZone
			if err := json.NewDecoder(r.Body).Decode(&zone); err != nil {
				writeError(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			zone.ID = id
			if err := d.zoneManager.Update(&zone); err != nil {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, zone)
		} else {
			http.NotFound(w, r)
		}

	case http.MethodDelete:
		if id != "" {
			if !d.zoneManager.Delete(id) {
				writeError(w, http.StatusNotFound, "intrusion zone not found")
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		} else {
			http.NotFound(w, r)
		}

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
