package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

type ForensicsSearchParams struct {
	Cameras       []string `json:"cameras"`
	StartTime     string   `json:"start_time"`
	EndTime       string   `json:"end_time"`
	ObjectClasses []string `json:"object_classes"`
	Colors        []string `json:"colors"`
	Direction     string   `json:"direction"`
	MinConfidence float64  `json:"min_confidence"`
	ZoneIDs       []string `json:"zone_ids"`
	TrackID       string   `json:"track_id"`
	QueryText     string   `json:"query_text"`
	Limit         int      `json:"limit"`
	Offset        int      `json:"offset"`
	TenantID      string   `json:"-"`
}

type ForensicsResult struct {
	EventID     string    `json:"event_id"`
	CameraID    string    `json:"camera_id"`
	Timestamp   time.Time `json:"timestamp"`
	TrackID     string    `json:"track_id"`
	Class       string    `json:"class"`
	Confidence  float64   `json:"confidence"`
	BBox        []float64 `json:"bbox"`
	ThumbnailURL string   `json:"thumbnail_url,omitempty"`
	Metadata    string    `json:"metadata,omitempty"`
}

type TrackPoint struct {
	CameraID   string    `json:"camera_id"`
	Timestamp  time.Time `json:"timestamp"`
	BBox       []float64 `json:"bbox"`
}

type ForensicsService struct {
	db     *sqlx.DB
	logger *slog.Logger
}

func NewForensicsService(db *sqlx.DB, logger *slog.Logger) *ForensicsService {
	return &ForensicsService{
		db:     db,
		logger: logger,
	}
}

func (s *ForensicsService) SearchByAttributes(params ForensicsSearchParams) ([]ForensicsResult, int, error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if len(params.Cameras) > 0 {
		placeholders := make([]string, len(params.Cameras))
		for i, cam := range params.Cameras {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, cam)
			argIdx++
		}
		where += fmt.Sprintf(" AND camera_id IN (%s)", strings.Join(placeholders, ","))
	}

	if params.StartTime != "" {
		where += fmt.Sprintf(" AND event_time >= $%d", argIdx)
		args = append(args, params.StartTime)
		argIdx++
	}
	if params.EndTime != "" {
		where += fmt.Sprintf(" AND event_time <= $%d", argIdx)
		args = append(args, params.EndTime)
		argIdx++
	}

	if len(params.ObjectClasses) > 0 {
		placeholders := make([]string, len(params.ObjectClasses))
		for i, cls := range params.ObjectClasses {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, cls)
			argIdx++
		}
		where += fmt.Sprintf(" AND object_type IN (%s)", strings.Join(placeholders, ","))
	}

	if params.MinConfidence > 0 {
		where += fmt.Sprintf(" AND confidence >= $%d", argIdx)
		args = append(args, params.MinConfidence)
		argIdx++
	}

	if params.TrackID != "" {
		where += fmt.Sprintf(" AND track_id = $%d", argIdx)
		args = append(args, params.TrackID)
		argIdx++
	}

	if params.TenantID != "" {
		where += fmt.Sprintf(" AND camera_id IN (SELECT c.id FROM cameras c JOIN sites s ON c.site_id = s.id WHERE s.tenant_id = $%d)", argIdx)
		args = append(args, params.TenantID)
		argIdx++
	}

	limit := 100
	offset := 0
	if params.Limit > 0 && params.Limit <= 1000 {
		limit = params.Limit
	}
	if params.Offset >= 0 {
		offset = params.Offset
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM ai_events " + where
	if err := s.db.Get(&total, countQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("count query: %w", err)
	}

	query := fmt.Sprintf(
		`SELECT id, camera_id, event_time, COALESCE(track_id,'') as track_id,
		        COALESCE(object_type,'') as object_type, confidence, bounding_box
		 FROM ai_events %s ORDER BY event_time DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.db.Queryx(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	var results []ForensicsResult
	for rows.Next() {
		var r struct {
			ID          string          `db:"id"`
			CameraID    string          `db:"camera_id"`
			EventTime   time.Time       `db:"event_time"`
			TrackID     string          `db:"track_id"`
			ObjectType  string          `db:"object_type"`
			Confidence  float64         `db:"confidence"`
			BoundingBox json.RawMessage `db:"bounding_box"`
		}
		if err := rows.StructScan(&r); err != nil {
			s.logger.Error("scan row", "error", err)
			continue
		}

		var bbox []float64
		json.Unmarshal(r.BoundingBox, &bbox)

		results = append(results, ForensicsResult{
			EventID:    r.ID,
			CameraID:   r.CameraID,
			Timestamp:  r.EventTime,
			TrackID:    r.TrackID,
			Class:      r.ObjectType,
			Confidence: r.Confidence,
			BBox:       bbox,
		})
	}

	if results == nil {
		results = []ForensicsResult{}
	}

	return results, total, nil
}

func (s *ForensicsService) SearchByVector(queryEmbedding []float32, limit int, tenantID string) ([]ForensicsResult, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	embJSON, err := json.Marshal(queryEmbedding)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding: %w", err)
	}

	query := `SELECT id, camera_id, event_time, COALESCE(track_id,'') as track_id,
		        COALESCE(object_type,'') as object_type, confidence, bounding_box,
		        1 - (embedding <=> $1::vector) as similarity
		 FROM ai_events
		 WHERE embedding IS NOT NULL`
	args := []interface{}{string(embJSON)}
	if tenantID != "" {
		query += fmt.Sprintf(" AND camera_id IN (SELECT c.id FROM cameras c JOIN sites s ON c.site_id = s.id WHERE s.tenant_id = $2)")
		args = append(args, tenantID)
	}
	query += ` ORDER BY embedding <=> $1::vector LIMIT $3`
	args = append(args, limit)

	rows, err := s.db.Queryx(query, args...)
	if err != nil {
		return nil, fmt.Errorf("vector search query: %w", err)
	}
	defer rows.Close()

	var results []ForensicsResult
	for rows.Next() {
		var r struct {
			ID          string          `db:"id"`
			CameraID    string          `db:"camera_id"`
			EventTime   time.Time       `db:"event_time"`
			TrackID     string          `db:"track_id"`
			ObjectType  string          `db:"object_type"`
			Confidence  float64         `db:"confidence"`
			BoundingBox json.RawMessage `db:"bounding_box"`
		}
		if err := rows.StructScan(&r); err != nil {
			s.logger.Error("scan vector row", "error", err)
			continue
		}

		var bbox []float64
		json.Unmarshal(r.BoundingBox, &bbox)

		results = append(results, ForensicsResult{
			EventID:    r.ID,
			CameraID:   r.CameraID,
			Timestamp:  r.EventTime,
			TrackID:    r.TrackID,
			Class:      r.ObjectType,
			Confidence: r.Confidence,
			BBox:       bbox,
		})
	}

	if results == nil {
		results = []ForensicsResult{}
	}

	return results, nil
}

func (s *ForensicsService) GetTrackPath(trackID string, tenantID string) ([]TrackPoint, error) {
	query := `SELECT e.camera_id, e.event_time, e.bounding_box
		FROM ai_events e`
	args := []interface{}{trackID}
	if tenantID != "" {
		query += ` JOIN cameras c ON e.camera_id = c.id
		JOIN sites s ON c.site_id = s.id
		WHERE e.track_id = $1 AND s.tenant_id = $2`
		args = append(args, tenantID)
	} else {
		query += ` WHERE e.track_id = $1`
	}
	query += ` ORDER BY e.event_time ASC`

	rows, err := s.db.Queryx(query, args...)
	if err != nil {
		return nil, fmt.Errorf("track path query: %w", err)
	}
	defer rows.Close()

	var path []TrackPoint
	for rows.Next() {
		var tp struct {
			CameraID    string          `db:"camera_id"`
			EventTime   time.Time       `db:"event_time"`
			BoundingBox json.RawMessage `db:"bounding_box"`
		}
		if err := rows.StructScan(&tp); err != nil {
			s.logger.Error("scan track point", "error", err)
			continue
		}

		var bbox []float64
		json.Unmarshal(tp.BoundingBox, &bbox)

		path = append(path, TrackPoint{
			CameraID:  tp.CameraID,
			Timestamp: tp.EventTime,
			BBox:      bbox,
		})
	}

	if path == nil {
		path = []TrackPoint{}
	}

	return path, nil
}

func (s *ForensicsService) ExportJSON(results []ForensicsResult) ([]byte, error) {
	return json.MarshalIndent(results, "", "  ")
}

func (s *ForensicsService) ExportCSV(results []ForensicsResult) ([]byte, error) {
	var buf strings.Builder
	writer := csv.NewWriter(&buf)

	writer.Write([]string{"event_id", "camera_id", "timestamp", "track_id", "class", "confidence", "bbox"})

	for _, r := range results {
		bboxStr, _ := json.Marshal(r.BBox)
		writer.Write([]string{
			r.EventID,
			r.CameraID,
			r.Timestamp.Format(time.RFC3339),
			r.TrackID,
			r.Class,
			fmt.Sprintf("%.4f", r.Confidence),
			string(bboxStr),
		})
	}

	writer.Flush()
	return []byte(buf.String()), nil
}

func (s *ForensicsService) HandleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var params ForensicsSearchParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	tenantID, _ := r.Context().Value("tenant_id").(string)
	params.TenantID = tenantID

	results, total, err := s.SearchByAttributes(params)
	if err != nil {
		s.logger.Error("forensics search failed", "error", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"results": results,
		"total":   total,
	})
}

func (s *ForensicsService) HandleVectorSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Embedding []float64 `json:"embedding"`
		Limit     int       `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if len(req.Embedding) == 0 {
		writeError(w, http.StatusBadRequest, "embedding required")
		return
	}

	emb32 := make([]float32, len(req.Embedding))
	for i, v := range req.Embedding {
		emb32[i] = float32(v)
	}

	tenantID, _ := r.Context().Value("tenant_id").(string)
	results, err := s.SearchByVector(emb32, req.Limit, tenantID)
	if err != nil {
		s.logger.Error("vector search failed", "error", err)
		writeError(w, http.StatusInternalServerError, "vector search failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"results": results,
		"total":   len(results),
	})
}

func (s *ForensicsService) HandleTrackPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	trackID := strings.TrimPrefix(r.URL.Path, "/api/forensics/tracks/")
	if trackID == "" {
		writeError(w, http.StatusBadRequest, "track_id required")
		return
	}

	tenantID, _ := r.Context().Value("tenant_id").(string)
	path, err := s.GetTrackPath(trackID, tenantID)
	if err != nil {
		s.logger.Error("track path failed", "error", err)
		writeError(w, http.StatusInternalServerError, "track path query failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"track_id": trackID,
		"path":     path,
	})
}

func (s *ForensicsService) HandleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Params ForensicsSearchParams `json:"params"`
		Format string                `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	tenantID, _ := r.Context().Value("tenant_id").(string)
	req.Params.TenantID = tenantID

	results, _, err := s.SearchByAttributes(req.Params)
	if err != nil {
		s.logger.Error("export search failed", "error", err)
		writeError(w, http.StatusInternalServerError, "export failed")
		return
	}

	switch req.Format {
	case "csv":
		data, err := s.ExportCSV(results)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "csv export failed")
			return
		}
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=forensics_export.csv")
		w.Write(data)

	case "json":
		fallthrough
	default:
		data, err := s.ExportJSON(results)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "json export failed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=forensics_export.json")
		w.Write(data)
	}
}
