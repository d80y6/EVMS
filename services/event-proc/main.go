package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"github.com/dam-vms/dam/pkg/common"
)

type EventProcConfig struct {
	NATSURL                   string
	PersonConfidenceThreshold float64
}

func DefaultEventProcConfig() *EventProcConfig {
	return &EventProcConfig{
		NATSURL:                   common.GetEnv("NATS_URL", "nats://nats:4222"),
		PersonConfidenceThreshold: 0.8,
	}
}

type Detection struct {
	Label      string    `json:"label"`
	Confidence float64   `json:"confidence"`
	BBox       []float64 `json:"bbox"`
}

type Track struct {
	TrackID    string    `json:"track_id"`
	Label      string    `json:"label"`
	Confidence float64   `json:"confidence"`
	BBox       []float64 `json:"bbox"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
	CameraID   string    `json:"camera_id"`
}

type Tracker struct {
	mu           sync.Mutex
	tracks       map[string][]*Track
	ioUThreshold float64
	trackTimeout time.Duration
}

func NewTracker(ioUThreshold float64, trackTimeout time.Duration) *Tracker {
	return &Tracker{
		tracks:       make(map[string][]*Track),
		ioUThreshold: ioUThreshold,
		trackTimeout: trackTimeout,
	}
}

func (t *Tracker) computeIoU(box1, box2 []float64) float64 {
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

func (t *Tracker) matchDetections(cameraID string, detections []Detection) []Track {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()

	if t.tracks[cameraID] == nil {
		t.tracks[cameraID] = make([]*Track, 0)
	}

	var matchedTracks []Track
	usedTracks := make(map[int]bool)

	for _, d := range detections {
		bestIdx := -1
		bestIoU := t.ioUThreshold

		for i, tr := range t.tracks[cameraID] {
			if usedTracks[i] {
				continue
			}
			if tr.Label != d.Label {
				continue
			}
			iou := t.computeIoU(tr.BBox, d.BBox)
			if iou > bestIoU {
				bestIoU = iou
				bestIdx = i
			}
		}

		if bestIdx >= 0 {
			usedTracks[bestIdx] = true
			tr := t.tracks[cameraID][bestIdx]
			tr.BBox = d.BBox
			tr.Confidence = d.Confidence
			tr.LastSeen = now

			matchedTracks = append(matchedTracks, Track{
				TrackID:    tr.TrackID,
				Label:      tr.Label,
				Confidence: tr.Confidence,
				BBox:       tr.BBox,
				FirstSeen:  tr.FirstSeen,
				LastSeen:   tr.LastSeen,
				CameraID:   cameraID,
			})
		} else {
			track := &Track{
				TrackID:    uuid.New().String(),
				Label:      d.Label,
				Confidence: d.Confidence,
				BBox:       d.BBox,
				FirstSeen:  now,
				LastSeen:   now,
				CameraID:   cameraID,
			}
			t.tracks[cameraID] = append(t.tracks[cameraID], track)

			matchedTracks = append(matchedTracks, Track{
				TrackID:    track.TrackID,
				Label:      track.Label,
				Confidence: track.Confidence,
				BBox:       track.BBox,
				FirstSeen:  track.FirstSeen,
				LastSeen:   track.LastSeen,
				CameraID:   cameraID,
			})
		}
	}

	return matchedTracks
}

func (t *Tracker) cleanupStaleTracks() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()

	for cameraID, tracks := range t.tracks {
		active := tracks[:0]
		for _, tr := range tracks {
			if now.Sub(tr.LastSeen) < t.trackTimeout {
				active = append(active, tr)
			}
		}
		t.tracks[cameraID] = active
	}
}

func (t *Tracker) StartCleanupLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.cleanupStaleTracks()
		case <-stopCh:
			return
		}
	}
}

type EventProcessor struct {
	config   *EventProcConfig
	nc       *nats.Conn
	logger   *slog.Logger
	eventSub *nats.Subscription
	tracker  *Tracker
	stopCh   chan struct{}
}

func NewEventProcessor(config *EventProcConfig, logger *slog.Logger) (*EventProcessor, error) {
	nc, err := nats.Connect(config.NATSURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	return &EventProcessor{
		config:  config,
		nc:      nc,
		logger:  logger,
		tracker: NewTracker(0.3, 3*time.Second),
		stopCh:  make(chan struct{}),
	}, nil
}

func (s *EventProcessor) Start() error {
	var err error
	s.eventSub, err = s.nc.QueueSubscribe("camera.*.events", "event-proc", s.handleCameraEvent, nats.PendingLimits(1024, 64*1024*1024))
	if err != nil {
		return fmt.Errorf("failed to subscribe to camera events: %w", err)
	}

	go s.tracker.StartCleanupLoop(s.stopCh)

	s.logger.Info("Event Processing Service started",
		"person_confidence_threshold", s.config.PersonConfidenceThreshold)
	return nil
}

func (s *EventProcessor) handleCameraEvent(msg *nats.Msg) {
	var detections []Detection
	if err := json.Unmarshal(msg.Data, &detections); err != nil {
		s.logger.Error("Failed to unmarshal detections", "error", err)
		return
	}

	cameraID := extractCameraID(msg.Subject)

	tracks := s.tracker.matchDetections(cameraID, detections)

	if len(tracks) > 0 {
		trackData, err := json.Marshal(tracks)
		if err != nil {
			s.logger.Error("Failed to marshal tracks", "error", err)
		} else {
			trackSubject := fmt.Sprintf("camera.%s.tracks", cameraID)
			if err := s.nc.Publish(trackSubject, trackData); err != nil {
				s.logger.Error("Failed to publish tracks", "error", err)
			}
		}
	}

	for _, d := range detections {
		if s.shouldTriggerNotification(d) {
			s.triggerNotification(msg.Subject, d)
		}
	}
}

func (s *EventProcessor) shouldTriggerNotification(d Detection) bool {
	return d.Label == "person" && d.Confidence > s.config.PersonConfidenceThreshold
}

func (s *EventProcessor) triggerNotification(subject string, detection Detection) {
	cameraID := extractCameraID(subject)

	s.logger.Info("Person detected! Triggering notification.",
		"camera", subject,
		"camera_id", cameraID,
		"confidence", detection.Confidence)

	notification := map[string]string{
		"title":   "Security Alert",
		"message": fmt.Sprintf("Person detected on camera %s with %.0f%% confidence", cameraID, detection.Confidence*100),
	}

	data, err := json.Marshal(notification)
	if err != nil {
		s.logger.Error("Failed to marshal notification", "error", err)
		return
	}

	if err := s.nc.Publish("notifications.push", data); err != nil {
		s.logger.Error("Failed to publish notification", "error", err)
	}
}

func extractCameraID(subject string) string {
	parts := strings.Split(subject, ".")
	if len(parts) >= 2 {
		return parts[1]
	}
	return "unknown"
}

func (s *EventProcessor) Close() error {
	close(s.stopCh)

	var errs []error

	if s.eventSub != nil {
		if err := s.eventSub.Unsubscribe(); err != nil {
			errs = append(errs, fmt.Errorf("failed to unsubscribe: %w", err))
		}
	}

	if s.nc != nil {
		s.nc.Close()
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during shutdown: %w", errs)
	}
	return nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultEventProcConfig()

	common.StartMetricsServer(common.GetEnv("METRICS_ADDR", ":2112"))

	service, err := NewEventProcessor(config, logger)
	if err != nil {
		logger.Error("Failed to initialize event processor", "error", err)
		os.Exit(1)
	}
	defer service.Close()

	if err := service.Start(); err != nil {
		logger.Error("Failed to start event processor", "error", err)
		os.Exit(1)
	}

	<-ctx.Done()
	logger.Info("Shutting down Event Processing Service...")
}
