package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
)

type FrameIndex struct {
	ID          string                 `json:"id" db:"id"`
	CameraID    string                 `json:"camera_id" db:"camera_id"`
	OffsetBytes int64                  `json:"offset_bytes" db:"offset_bytes"`
	Timestamp   time.Time              `json:"timestamp" db:"timestamp"`
	MotionScore float64                `json:"motion_score" db:"motion_score"`
	SceneChange bool                   `json:"scene_change" db:"scene_change"`
	Metadata    map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
}

type FrameAnalysisService struct {
	db      *sqlx.DB
	enabled bool
}

func NewFrameAnalysisService(db *sqlx.DB) *FrameAnalysisService {
	return &FrameAnalysisService{db: db, enabled: true}
}

func (s *FrameAnalysisService) IndexFrame(ctx context.Context, frame *FrameIndex) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO frame_index (camera_id, offset_bytes, timestamp, motion_score, scene_change, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		frame.CameraID, frame.OffsetBytes, frame.Timestamp, frame.MotionScore, frame.SceneChange, frame.Metadata)
	return err
}

func (s *FrameAnalysisService) GetFrameAt(ctx context.Context, cameraID string, ts time.Time) (*FrameIndex, error) {
	var f FrameIndex
	err := s.db.GetContext(ctx, &f,
		"SELECT id, camera_id, offset_bytes, timestamp, motion_score, scene_change, metadata FROM frame_index WHERE camera_id = $1 AND timestamp <= $2 ORDER BY timestamp DESC LIMIT 1",
		cameraID, ts)
	if err != nil {
		return nil, fmt.Errorf("frame not found: %w", err)
	}
	return &f, nil
}

func (s *FrameAnalysisService) GetMotionFrames(ctx context.Context, cameraID string, start, end time.Time, minScore float64) ([]FrameIndex, error) {
	var frames []FrameIndex
	err := s.db.SelectContext(ctx, &frames,
		"SELECT id, camera_id, offset_bytes, timestamp, motion_score, scene_change, metadata FROM frame_index WHERE camera_id = $1 AND timestamp >= $2 AND timestamp <= $3 AND motion_score >= $4 ORDER BY timestamp",
		cameraID, start, end, minScore)
	if err != nil {
		return nil, err
	}
	if frames == nil {
		frames = []FrameIndex{}
	}
	return frames, nil
}

func (s *FrameAnalysisService) GetSceneChanges(ctx context.Context, cameraID string, start, end time.Time) ([]FrameIndex, error) {
	var frames []FrameIndex
	err := s.db.SelectContext(ctx, &frames,
		"SELECT id, camera_id, offset_bytes, timestamp, motion_score, scene_change, metadata FROM frame_index WHERE camera_id = $1 AND timestamp >= $2 AND timestamp <= $3 AND scene_change = true ORDER BY timestamp",
		cameraID, start, end)
	if err != nil {
		return nil, err
	}
	if frames == nil {
		frames = []FrameIndex{}
	}
	return frames, nil
}

func computeMotionScore(prev []byte, curr []byte) float64 {
	if len(prev) == 0 || len(curr) == 0 {
		return 0
	}
	minLen := len(prev)
	if len(curr) < minLen {
		minLen = len(curr)
	}
	var diffSum float64
	pixels := 0
	for i := 0; i < minLen-1; i += 2 {
		diff := float64(curr[i]) - float64(prev[i])
		diffSum += math.Abs(diff)
		pixels++
	}
	if pixels == 0 {
		return 0
	}
	score := diffSum / float64(pixels) / 255.0 * 100
	if score > 100 {
		score = 100
	}
	return score
}

func handleGetFrameAt(fas *FrameAnalysisService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cameraID := r.URL.Query().Get("camera_id")
		tsStr := r.URL.Query().Get("timestamp")

		if cameraID == "" || tsStr == "" {
			jsonError(w, "camera_id and timestamp are required", http.StatusBadRequest)
			return
		}

		ts, err := time.Parse(time.RFC3339, tsStr)
		if err != nil {
			jsonError(w, "invalid timestamp, use RFC3339 format", http.StatusBadRequest)
			return
		}

		frame, err := fas.GetFrameAt(r.Context(), cameraID, ts)
		if err != nil {
			jsonError(w, "frame not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(frame)
	}
}

func handleGetMotionFrames(fas *FrameAnalysisService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cameraID := r.URL.Query().Get("camera_id")
		startStr := r.URL.Query().Get("start")
		endStr := r.URL.Query().Get("end")
		minScore := 0.1

		if s := r.URL.Query().Get("min_score"); s != "" {
			fmt.Sscanf(s, "%f", &minScore)
		}

		if cameraID == "" || startStr == "" || endStr == "" {
			jsonError(w, "camera_id, start, and end are required", http.StatusBadRequest)
			return
		}

		start, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			jsonError(w, "invalid start time", http.StatusBadRequest)
			return
		}
		end, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			jsonError(w, "invalid end time", http.StatusBadRequest)
			return
		}

		frames, err := fas.GetMotionFrames(r.Context(), cameraID, start, end, minScore)
		if err != nil {
			jsonError(w, "failed to get motion frames: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"camera_id": cameraID,
			"frames":    frames,
			"count":     len(frames),
		})
	}
}

func handleGetSceneChanges(fas *FrameAnalysisService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cameraID := r.URL.Query().Get("camera_id")
		startStr := r.URL.Query().Get("start")
		endStr := r.URL.Query().Get("end")

		if cameraID == "" || startStr == "" || endStr == "" {
			jsonError(w, "camera_id, start, and end are required", http.StatusBadRequest)
			return
		}

		start, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			jsonError(w, "invalid start time", http.StatusBadRequest)
			return
		}
		end, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			jsonError(w, "invalid end time", http.StatusBadRequest)
			return
		}

		frames, err := fas.GetSceneChanges(r.Context(), cameraID, start, end)
		if err != nil {
			jsonError(w, "failed to get scene changes: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"camera_id": cameraID,
			"frames":    frames,
			"count":     len(frames),
		})
	}
}
