package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
)

type HeatmapCell struct {
	CameraID string    `json:"camera_id"`
	X        int       `json:"x"`
	Y        int       `json:"y"`
	Count    int       `json:"count"`
	Bucket   time.Time `json:"bucket"`
}

type HeatmapAggregator struct {
	db         *sqlx.DB
	gridSize   int
	cellWidth  float64
	cellHeight float64
}

func NewHeatmapAggregator(db *sqlx.DB) *HeatmapAggregator {
	return &HeatmapAggregator{
		db:         db,
		gridSize:   20,
		cellWidth:  1.0 / 20,
		cellHeight: 1.0 / 20,
	}
}

func (ha *HeatmapAggregator) Init(ctx context.Context) error {
	_, err := ha.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS crowd_heatmaps (
			camera_id UUID NOT NULL,
			cell_x INT NOT NULL,
			cell_y INT NOT NULL,
			bucket TIMESTAMPTZ NOT NULL,
			count INT NOT NULL DEFAULT 0,
			PRIMARY KEY (camera_id, cell_x, cell_y, bucket)
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to create crowd_heatmaps table: %w", err)
	}
	ha.db.ExecContext(ctx, "SELECT create_hypertable('crowd_heatmaps', 'bucket', if_not_exists => true)")
	return nil
}

func (ha *HeatmapAggregator) RecordDetection(cameraID string, bbox [4]float64, t time.Time) {
	centerX := (bbox[0] + bbox[2]) / 2
	centerY := (bbox[1] + bbox[3]) / 2

	cellX := int(centerX / ha.cellWidth)
	cellY := int(centerY / ha.cellHeight)

	if cellX >= ha.gridSize {
		cellX = ha.gridSize - 1
	}
	if cellY >= ha.gridSize {
		cellY = ha.gridSize - 1
	}
	if cellX < 0 {
		cellX = 0
	}
	if cellY < 0 {
		cellY = 0
	}

	bucket := t.Truncate(1 * time.Hour)

	ha.db.Exec(
		`INSERT INTO crowd_heatmaps (camera_id, cell_x, cell_y, bucket, count)
		 VALUES ($1, $2, $3, $4, 1)
		 ON CONFLICT (camera_id, cell_x, cell_y, bucket)
		 DO UPDATE SET count = crowd_heatmaps.count + 1`,
		cameraID, cellX, cellY, bucket)
}

func (ha *HeatmapAggregator) GetHeatmap(cameraID string, start, end time.Time) ([]HeatmapCell, error) {
	var cells []HeatmapCell
	err := ha.db.Select(&cells,
		`SELECT camera_id, cell_x, cell_y, SUM(count) as count, bucket
		 FROM crowd_heatmaps
		 WHERE camera_id=$1 AND bucket BETWEEN $2 AND $3
		 GROUP BY camera_id, cell_x, cell_y, bucket
		 ORDER BY bucket`,
		cameraID, start, end)
	return cells, err
}

func handleHeatmap(ha *HeatmapAggregator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cameraID := r.URL.Query().Get("camera_id")
		if cameraID == "" {
			writeError(w, http.StatusBadRequest, "camera_id required")
			return
		}

		end := time.Now()
		start := end.Add(-24 * time.Hour)
		if s := r.URL.Query().Get("start"); s != "" {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				start = t
			}
		}
		if e := r.URL.Query().Get("end"); e != "" {
			if t, err := time.Parse(time.RFC3339, e); err == nil {
				end = t
			}
		}

		cells, err := ha.GetHeatmap(cameraID, start, end)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to get heatmap")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"cells": cells})
	}
}

func handlePeopleCounts(db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var counts []struct {
			CameraID string `json:"camera_id" db:"camera_id"`
			ZoneID   string `json:"zone_id" db:"zone_id"`
			Count    int    `json:"count" db:"count"`
		}
		err := db.Select(&counts, `SELECT camera_id, zone_id, SUM(count)::int AS count FROM people_counters GROUP BY camera_id, zone_id`)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to get people counts")
			return
		}
		if counts == nil {
			counts = []struct {
				CameraID string `json:"camera_id" db:"camera_id"`
				ZoneID   string `json:"zone_id" db:"zone_id"`
				Count    int    `json:"count" db:"count"`
			}{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"counts": counts})
	}
}
