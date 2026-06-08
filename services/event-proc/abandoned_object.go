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
)

type AbandonedObjectZone struct {
	ID                       string    `json:"id"`
	CameraID                 string    `json:"camera_id"`
	Name                     string    `json:"name"`
	PolygonPoints            []Point   `json:"polygon_points"`
	StationaryThresholdSeconds int     `json:"stationary_threshold_seconds"`
	Enabled                  bool      `json:"enabled"`
	CreatedAt                time.Time `json:"created_at"`
}

type AbandonedObjectEvent struct {
	ID            string    `json:"id"`
	ZoneID        string    `json:"zone_id"`
	CameraID      string    `json:"camera_id"`
	ObjectID      string    `json:"object_id"`
	ObjectClass   string    `json:"object_class"`
	StationarySeconds float64 `json:"stationary_seconds"`
	Confidence    float64   `json:"confidence"`
	BBox          []float64 `json:"bbox"`
	Timestamp     time.Time `json:"timestamp"`
}

type StationaryObject struct {
	ObjectID    string    `json:"object_id"`
	ZoneID      string    `json:"zone_id"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	BBox        []float64 `json:"bbox"`
	Class       string    `json:"class"`
	Confidence  float64   `json:"confidence"`
	CameraID    string    `json:"camera_id"`
}

type AbandonedObjectZoneManager struct {
	mu    sync.RWMutex
	zones map[string]*AbandonedObjectZone
}

func NewAbandonedObjectZoneManager() *AbandonedObjectZoneManager {
	return &AbandonedObjectZoneManager{
		zones: make(map[string]*AbandonedObjectZone),
	}
}

func (m *AbandonedObjectZoneManager) List(cameraID string) []*AbandonedObjectZone {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*AbandonedObjectZone
	for _, z := range m.zones {
		if cameraID == "" || z.CameraID == cameraID {
			result = append(result, z)
		}
	}
	return result
}

func (m *AbandonedObjectZoneManager) Get(id string) (*AbandonedObjectZone, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	z, ok := m.zones[id]
	if !ok {
		return nil, fmt.Errorf("abandoned object zone not found: %s", id)
	}
	return z, nil
}

func (m *AbandonedObjectZoneManager) Create(zone *AbandonedObjectZone) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	zone.ID = uuid.New().String()
	zone.CreatedAt = time.Now().UTC()
	if zone.StationaryThresholdSeconds == 0 {
		zone.StationaryThresholdSeconds = 60
	}
	m.zones[zone.ID] = zone
	return nil
}

func (m *AbandonedObjectZoneManager) Update(zone *AbandonedObjectZone) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.zones[zone.ID]
	if !ok {
		return fmt.Errorf("abandoned object zone not found: %s", zone.ID)
	}
	zone.CreatedAt = existing.CreatedAt
	m.zones[zone.ID] = zone
	return nil
}

func (m *AbandonedObjectZoneManager) Delete(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.zones[id]
	if !ok {
		return false
	}
	delete(m.zones, id)
	return true
}

func (m *AbandonedObjectZoneManager) GetActive(cameraID string) []*AbandonedObjectZone {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*AbandonedObjectZone
	for _, z := range m.zones {
		if z.Enabled && (cameraID == "" || z.CameraID == cameraID) {
			result = append(result, z)
		}
	}
	return result
}

type objectKey struct {
	CameraID string
	CenterX  int
	CenterY  int
	Class    string
}

type AbandonedObjectDetector struct {
	zoneManager  *AbandonedObjectZoneManager
	objects      map[string]*StationaryObject
	objectKeyMap map[string]string
	mu           sync.Mutex
	logger       *slog.Logger
	stopCh       chan struct{}
	checkInterval time.Duration
	ioUThreshold float64
}

func NewAbandonedObjectDetector(logger *slog.Logger) *AbandonedObjectDetector {
	return &AbandonedObjectDetector{
		zoneManager:   NewAbandonedObjectZoneManager(),
		objects:       make(map[string]*StationaryObject),
		objectKeyMap:  make(map[string]string),
		logger:        logger,
		stopCh:        make(chan struct{}),
		checkInterval: 5 * time.Second,
		ioUThreshold:  0.85,
	}
}

func (d *AbandonedObjectDetector) Start() {
	go d.stationaryCheckLoop()
	go d.cleanupLoop()
}

func (d *AbandonedObjectDetector) Stop() {
	close(d.stopCh)
}

func (d *AbandonedObjectDetector) cleanupLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			d.mu.Lock()
			now := time.Now()
			for id, obj := range d.objects {
				if now.Sub(obj.LastSeen) > 30*time.Second {
					delete(d.objects, id)
				}
			}
			d.mu.Unlock()
		case <-d.stopCh:
			return
		}
	}
}

func computeIoU(box1, box2 []float64) float64 {
	if len(box1) < 4 || len(box2) < 4 {
		return 0
	}
	x1 := math.Max(box1[0], box2[0])
	y1 := math.Max(box1[1], box2[1])
	x2 := math.Min(box1[2], box2[2])
	y2 := math.Min(box1[3], box2[3])

	if x2 <= x1 || y2 <= y1 {
		return 0
	}

	interArea := (x2 - x1) * (y2 - y1)
	box1Area := (box1[2] - box1[0]) * (box1[3] - box1[1])
	box2Area := (box2[2] - box2[0]) * (box2[3] - box2[1])
	unionArea := box1Area + box2Area - interArea

	if unionArea <= 0 {
		return 0
	}
	return interArea / unionArea
}

func (d *AbandonedObjectDetector) Evaluate(cameraID string, detections []Detection) []StationaryObject {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	used := make(map[string]bool)

	for _, det := range detections {
		if len(det.BBox) < 4 {
			continue
		}
		bestID := ""
		bestIoU := d.ioUThreshold

		for id, obj := range d.objects {
			if obj.CameraID != cameraID || obj.Class != det.Label {
				continue
			}
			iou := computeIoU(obj.BBox, det.BBox)
			if iou > bestIoU {
				bestIoU = iou
				bestID = id
			}
		}

		if bestID != "" {
			obj := d.objects[bestID]
			obj.BBox = det.BBox
			obj.LastSeen = now
			obj.Confidence = det.Confidence
			used[bestID] = true
		} else {
			objID := uuid.New().String()
			d.objects[objID] = &StationaryObject{
				ObjectID:   objID,
				FirstSeen:  now,
				LastSeen:   now,
				BBox:       det.BBox,
				Class:      det.Label,
				Confidence: det.Confidence,
				CameraID:   cameraID,
			}
			used[objID] = true
		}
	}

	var activeObjects []StationaryObject
	for _, obj := range d.objects {
		activeObjects = append(activeObjects, *obj)
	}

	return activeObjects
}

func (d *AbandonedObjectDetector) stationaryCheckLoop() {
	ticker := time.NewTicker(d.checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			d.mu.Lock()
			now := time.Now()
			for _, obj := range d.objects {
				stationary := now.Sub(obj.FirstSeen).Seconds()
				if stationary >= 30 {
					d.logger.Info("Stationary object detected",
						"object_id", obj.ObjectID,
						"class", obj.Class,
						"camera", obj.CameraID,
						"stationary_seconds", stationary)
				}
			}
			d.mu.Unlock()
		case <-d.stopCh:
			return
		}
	}
}

func (d *AbandonedObjectDetector) HandleHTTP(w http.ResponseWriter, r *http.Request) {
	id := ""
	path := r.URL.Path
	if strings.HasPrefix(path, "/api/abandoned-object-zones/") {
		id = path[len("/api/abandoned-object-zones/"):]
	}

	switch r.Method {
	case http.MethodGet:
		if id == "" {
			cameraID := r.URL.Query().Get("camera_id")
			zones := d.zoneManager.List(cameraID)
			if zones == nil {
				zones = []*AbandonedObjectZone{}
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
			var zone AbandonedObjectZone
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
			var zone AbandonedObjectZone
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
				writeError(w, http.StatusNotFound, "abandoned object zone not found")
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
