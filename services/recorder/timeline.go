package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
)

type TimelineEntryType string

const (
	TimelineRecording TimelineEntryType = "recording"
	TimelineEvent     TimelineEntryType = "event"
	TimelineBookmark  TimelineEntryType = "bookmark"
	TimelineMotion    TimelineEntryType = "motion"
)

type TimelineEntry struct {
	CameraID  string            `json:"camera_id" db:"camera_id"`
	Timestamp time.Time         `json:"timestamp" db:"timestamp"`
	Type      TimelineEntryType `json:"type"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type TimelineSegment struct {
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	HasVideo bool      `json:"has_video"`
	EventIDs []string  `json:"event_ids,omitempty"`
}

type TimelineBucket struct {
	Bucket   time.Time `json:"bucket"`
	Count    int       `json:"count"`
	HasVideo bool      `json:"has_video"`
	HasEvent bool      `json:"has_event"`
}

type TimelineService struct {
	db *sqlx.DB
}

func NewTimelineService(db *sqlx.DB) *TimelineService {
	return &TimelineService{db: db}
}

func (s *TimelineService) GetTimeline(cameraID string, start, end time.Time, granularity string) ([]TimelineBucket, error) {
	var trunc string
	switch granularity {
	case "hour":
		trunc = "hour"
	case "day":
		trunc = "day"
	default:
		trunc = "hour"
	}

	query := fmt.Sprintf(`
		SELECT
			bucket,
			COUNT(*) AS count,
			BOOL_OR(has_video) AS has_video,
			BOOL_OR(has_event) AS has_event
		FROM (
			SELECT
				date_trunc('%s', bucket) AS bucket,
				has_video,
				has_event
			FROM (
				SELECT
					generate_series(
						date_trunc('%[1]s', r.start_time),
						date_trunc('%[1]s', COALESCE(r.end_time, r.start_time + interval '1 hour')),
						'1 %[1]s'::interval
					) AS bucket,
					true AS has_video,
					false AS has_event
				FROM recordings r
				WHERE r.camera_id = $1 AND r.end_time >= $2 AND r.start_time <= $3
			) rec_exploded
			UNION ALL
			SELECT
				date_trunc('%[1]s', e.event_time) AS bucket,
				false AS has_video,
				true AS has_event
			FROM ai_events e
			WHERE e.camera_id = $1 AND e.event_time >= $2 AND e.event_time <= $3
		) combined
		GROUP BY bucket
		ORDER BY bucket
	`, trunc)

	var buckets []TimelineBucket
	err := s.db.Select(&buckets, query, cameraID, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to query timeline: %w", err)
	}
	if buckets == nil {
		buckets = []TimelineBucket{}
	}
	return buckets, nil
}

func (s *TimelineService) GetRecordingTimeline(cameraID string, start, end time.Time) ([]TimelineSegment, error) {
	var segments []struct {
		StartTime time.Time `db:"start_time"`
		EndTime   time.Time `db:"end_time"`
	}
	err := s.db.Select(&segments, `
		SELECT start_time, end_time
		FROM recordings
		WHERE camera_id = $1 AND end_time >= $2 AND start_time <= $3
		ORDER BY start_time
	`, cameraID, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to query recording timeline: %w", err)
	}

	timeline := make([]TimelineSegment, 0, len(segments))
	for _, seg := range segments {
		timeline = append(timeline, TimelineSegment{
			Start:    seg.StartTime,
			End:      seg.EndTime,
			HasVideo: true,
		})
	}
	return timeline, nil
}

func handleTimeline(ts *TimelineService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cameraID := r.URL.Query().Get("camera_id")
		startStr := r.URL.Query().Get("start")
		endStr := r.URL.Query().Get("end")
		granularity := r.URL.Query().Get("granularity")

		if cameraID == "" || startStr == "" || endStr == "" {
			jsonError(w, "camera_id, start, and end are required", http.StatusBadRequest)
			return
		}

		start, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			jsonError(w, "invalid start time, use RFC3339 format", http.StatusBadRequest)
			return
		}
		end, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			jsonError(w, "invalid end time, use RFC3339 format", http.StatusBadRequest)
			return
		}

		buckets, err := ts.GetTimeline(cameraID, start, end, granularity)
		if err != nil {
			jsonError(w, "failed to get timeline: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"camera_id":   cameraID,
			"start":       start,
			"end":         end,
			"granularity": granularity,
			"buckets":     buckets,
		})
	}
}

func handleRecordingTimeline(ts *TimelineService) http.HandlerFunc {
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

		segments, err := ts.GetRecordingTimeline(cameraID, start, end)
		if err != nil {
			jsonError(w, "failed to get recording timeline: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"camera_id": cameraID,
			"start":     start,
			"end":       end,
			"segments":  segments,
		})
	}
}
