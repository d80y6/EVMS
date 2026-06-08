package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type LoiteringZone struct {
	ID                   string    `json:"id"`
	CameraID             string    `json:"camera_id"`
	Name                 string    `json:"name"`
	PolygonPoints        []Point   `json:"polygon_points"`
	DwellThresholdSeconds int      `json:"dwell_threshold_seconds"`
	Enabled              bool      `json:"enabled"`
	CreatedAt            time.Time `json:"created_at"`
}

type LoiteringEvent struct {
	ID         string    `json:"id"`
	ZoneID     string    `json:"zone_id"`
	CameraID   string    `json:"camera_id"`
	TrackID    string    `json:"track_id"`
	DwellSeconds float64 `json:"dwell_seconds"`
	Confidence float64   `json:"confidence"`
	Timestamp  time.Time `json:"timestamp"`
}

type TrackDwellTime struct {
	TrackID   string    `json:"track_id"`
	ZoneID    string    `json:"zone_id"`
	EntryTime time.Time `json:"entry_time"`
	LastSeen  time.Time `json:"last_seen"`
	BBox      []float64 `json:"bbox"`
	Label     string    `json:"label"`
}

type LoiteringZoneManager struct {
	mu    sync.RWMutex
	zones map[string]*LoiteringZone
}

func NewLoiteringZoneManager() *LoiteringZoneManager {
	return &LoiteringZoneManager{
		zones: make(map[string]*LoiteringZone),
	}
}

func (m *LoiteringZoneManager) List(cameraID string) []*LoiteringZone {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*LoiteringZone
	for _, z := range m.zones {
		if cameraID == "" || z.CameraID == cameraID {
			result = append(result, z)
		}
	}
	return result
}

func (m *LoiteringZoneManager) Get(id string) (*LoiteringZone, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	z, ok := m.zones[id]
	if !ok {
		return nil, fmt.Errorf("loitering zone not found: %s", id)
	}
	return z, nil
}

func (m *LoiteringZoneManager) Create(zone *LoiteringZone) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	zone.ID = uuid.New().String()
	zone.CreatedAt = time.Now().UTC()
	if zone.DwellThresholdSeconds == 0 {
		zone.DwellThresholdSeconds = 30
	}
	m.zones[zone.ID] = zone
	return nil
}

func (m *LoiteringZoneManager) Update(zone *LoiteringZone) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.zones[zone.ID]
	if !ok {
		return fmt.Errorf("loitering zone not found: %s", zone.ID)
	}
	zone.CreatedAt = existing.CreatedAt
	m.zones[zone.ID] = zone
	return nil
}

func (m *LoiteringZoneManager) Delete(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.zones[id]
	if !ok {
		return false
	}
	delete(m.zones, id)
	return true
}

func (m *LoiteringZoneManager) GetActive(cameraID string) []*LoiteringZone {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*LoiteringZone
	for _, z := range m.zones {
		if z.Enabled && (cameraID == "" || z.CameraID == cameraID) {
			result = append(result, z)
		}
	}
	return result
}

type LoiteringDetector struct {
	zoneManager   *LoiteringZoneManager
	dwellTracks   map[string]*TrackDwellTime
	mu            sync.RWMutex
	logger        *slog.Logger
	stopCh        chan struct{}
	checkInterval time.Duration
}

func NewLoiteringDetector(logger *slog.Logger) *LoiteringDetector {
	return &LoiteringDetector{
		zoneManager:   NewLoiteringZoneManager(),
		dwellTracks:   make(map[string]*TrackDwellTime),
		logger:        logger,
		stopCh:        make(chan struct{}),
		checkInterval: 5 * time.Second,
	}
}

func (d *LoiteringDetector) Start() {
	go d.loiteringLoop()
	go d.cleanupLoop()
}

func (d *LoiteringDetector) Stop() {
	close(d.stopCh)
}

func (d *LoiteringDetector) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			d.mu.Lock()
			now := time.Now()
			for id, dt := range d.dwellTracks {
				if now.Sub(dt.LastSeen) > 10*time.Second {
					delete(d.dwellTracks, id)
				}
			}
			d.mu.Unlock()
		case <-d.stopCh:
			return
		}
	}
}

func (d *LoiteringDetector) Evaluate(cameraID string, tracks []Track, zones []*LoiteringZone) []LoiteringEvent {
	d.mu.Lock()
	now := time.Now()

	for _, tr := range tracks {
		c := centroid(tr.BBox)

		inZone := false
		for _, zone := range zones {
			if PointInPolygon(c, zone.PolygonPoints) {
				inZone = true
				key := tr.TrackID + ":" + zone.ID
				dt, exists := d.dwellTracks[key]
				if !exists {
					d.dwellTracks[key] = &TrackDwellTime{
						TrackID:   tr.TrackID,
						ZoneID:    zone.ID,
						EntryTime: now,
						LastSeen:  now,
						BBox:      tr.BBox,
						Label:     tr.Label,
					}
				} else {
					dt.LastSeen = now
					dt.BBox = tr.BBox
				}
			}
		}

		if !inZone {
			for key, dt := range d.dwellTracks {
				if dt.TrackID == tr.TrackID {
					delete(d.dwellTracks, key)
				}
			}
		}
	}
	d.mu.Unlock()

	var events []LoiteringEvent
	d.mu.RLock()
	for key, dt := range d.dwellTracks {
		zoneID := dt.ZoneID
		var zone *LoiteringZone
		for _, z := range zones {
			if z.ID == zoneID {
				zone = z
				break
			}
		}
		if zone == nil || !zone.Enabled {
			continue
		}
		dwell := now.Sub(dt.EntryTime).Seconds()
		if dwell >= float64(zone.DwellThresholdSeconds) {
			events = append(events, LoiteringEvent{
				ID:           uuid.New().String(),
				ZoneID:       zoneID,
				CameraID:     cameraID,
				TrackID:      dt.TrackID,
				DwellSeconds: dwell,
				Confidence:   1.0,
				Timestamp:    now,
			})
			delete(d.dwellTracks, key)
		}
	}
	d.mu.RUnlock()

	return events
}

func (d *LoiteringDetector) loiteringLoop() {
	ticker := time.NewTicker(d.checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			d.mu.Lock()
			now := time.Now()
			for key, dt := range d.dwellTracks {
				dwell := now.Sub(dt.EntryTime).Seconds()
				zones := d.zoneManager.GetActive(dt.TrackID)
				for _, z := range zones {
					if z.ID == dt.ZoneID && dwell >= float64(z.DwellThresholdSeconds) {
						d.logger.Info("Loitering detected",
							"track", dt.TrackID,
							"zone", z.Name,
							"dwell_seconds", dwell)
						delete(d.dwellTracks, key)
					}
				}
			}
			d.mu.Unlock()
		case <-d.stopCh:
			return
		}
	}
}

func (d *LoiteringDetector) HandleHTTP(w http.ResponseWriter, r *http.Request) {
	id := ""
	path := r.URL.Path
	if strings.HasPrefix(path, "/api/loitering-zones/") {
		id = path[len("/api/loitering-zones/"):]
	}

	switch r.Method {
	case http.MethodGet:
		if id == "" {
			cameraID := r.URL.Query().Get("camera_id")
			zones := d.zoneManager.List(cameraID)
			if zones == nil {
				zones = []*LoiteringZone{}
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
			var zone LoiteringZone
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
			var zone LoiteringZone
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
				writeError(w, http.StatusNotFound, "loitering zone not found")
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
