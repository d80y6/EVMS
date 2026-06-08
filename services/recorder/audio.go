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

type AudioMetadata struct {
	ID          string                 `json:"id" db:"id"`
	CameraID    string                 `json:"camera_id" db:"camera_id"`
	RecordingID *string                `json:"recording_id,omitempty" db:"recording_id"`
	StreamIndex int                    `json:"stream_index" db:"stream_index"`
	Codec       string                 `json:"codec" db:"codec"`
	SampleRate  int                    `json:"sample_rate" db:"sample_rate"`
	Channels    int                    `json:"channels" db:"channels"`
	HasAudio    bool                   `json:"has_audio" db:"has_audio"`
	RMSLevel    float64                `json:"rms_level" db:"rms_level"`
	PeakLevel   float64                `json:"peak_level" db:"peak_level"`
	Timestamp   time.Time              `json:"timestamp" db:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
}

type AudioService struct {
	db *sqlx.DB
}

func NewAudioService(db *sqlx.DB) *AudioService {
	return &AudioService{db: db}
}

func (s *AudioService) RecordAudioMetadata(ctx context.Context, meta *AudioMetadata) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO audio_metadata (camera_id, recording_id, stream_index, codec, sample_rate, channels, has_audio, rms_level, peak_level, timestamp, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		meta.CameraID, meta.RecordingID, meta.StreamIndex, meta.Codec, meta.SampleRate, meta.Channels, meta.HasAudio,
		meta.RMSLevel, meta.PeakLevel, meta.Timestamp, meta.Metadata)
	return err
}

func (s *AudioService) GetAudioMetadata(ctx context.Context, cameraID string, start, end time.Time) ([]AudioMetadata, error) {
	var meta []AudioMetadata
	err := s.db.SelectContext(ctx, &meta,
		"SELECT id, camera_id, recording_id, stream_index, codec, sample_rate, channels, has_audio, rms_level, peak_level, timestamp, metadata FROM audio_metadata WHERE camera_id = $1 AND timestamp >= $2 AND timestamp <= $3 ORDER BY timestamp",
		cameraID, start, end)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		meta = []AudioMetadata{}
	}
	return meta, nil
}

func (s *AudioService) GetCurrentAudioLevel(cameraID string) (*AudioMetadata, error) {
	var meta AudioMetadata
	err := s.db.Get(&meta,
		"SELECT id, camera_id, recording_id, stream_index, codec, sample_rate, channels, has_audio, rms_level, peak_level, timestamp, metadata FROM audio_metadata WHERE camera_id = $1 AND has_audio = true ORDER BY timestamp DESC LIMIT 1",
		cameraID)
	if err != nil {
		return nil, fmt.Errorf("no audio data found for camera: %w", err)
	}
	return &meta, nil
}

func computeRMSLevel(samples []int16) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sumSquares float64
	for _, s := range samples {
		f := float64(s) / 32768.0
		sumSquares += f * f
	}
	rms := math.Sqrt(sumSquares / float64(len(samples)))
	if rms > 0 {
		return 20 * math.Log10(rms)
	}
	return -100
}

func computePeakLevel(samples []int16) float64 {
	if len(samples) == 0 {
		return -100
	}
	var peak float64
	for _, s := range samples {
		f := math.Abs(float64(s)) / 32768.0
		if f > peak {
			peak = f
		}
	}
	if peak > 0 {
		return 20 * math.Log10(peak)
	}
	return -100
}

func handleRecordAudio(svc *AudioService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req AudioMetadata
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.CameraID == "" {
			jsonError(w, "camera_id is required", http.StatusBadRequest)
			return
		}

		req.Timestamp = time.Now().UTC()

		if err := svc.RecordAudioMetadata(r.Context(), &req); err != nil {
			jsonError(w, "failed to record audio metadata: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "recorded"})
	}
}

func handleGetAudioMetadata(svc *AudioService) http.HandlerFunc {
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

		meta, err := svc.GetAudioMetadata(r.Context(), cameraID, start, end)
		if err != nil {
			jsonError(w, "failed to get audio metadata: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"camera_id": cameraID,
			"metadata":  meta,
		})
	}
}

func handleGetAudioLevel(svc *AudioService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cameraID := r.URL.Query().Get("camera_id")
		if cameraID == "" {
			jsonError(w, "camera_id is required", http.StatusBadRequest)
			return
		}

		meta, err := svc.GetCurrentAudioLevel(cameraID)
		if err != nil {
			jsonError(w, "no audio data available", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(meta)
	}
}

func handleListAudioCameras(svc *AudioService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type audioCamera struct {
			CameraID  string    `db:"camera_id" json:"camera_id"`
			HasAudio  bool      `db:"has_audio" json:"has_audio"`
			Codec     string    `db:"codec" json:"codec"`
			UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
		}
		var cameras []audioCamera
		err := svc.db.Select(&cameras, `
			SELECT DISTINCT ON (camera_id) camera_id, has_audio, codec, timestamp AS updated_at
			FROM audio_metadata
			ORDER BY camera_id, timestamp DESC`)
		if err != nil {
			jsonError(w, "failed to list cameras: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if cameras == nil {
			cameras = []audioCamera{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"cameras": cameras})
	}
}
