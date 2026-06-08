package main

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/jmoiron/sqlx"
)

type StorageEstimate struct {
	CameraID         string  `json:"camera_id"`
	CameraName       string  `json:"camera_name"`
	RetentionDays    int     `json:"retention_days"`
	DailyUsageGB     float64 `json:"daily_usage_gb"`
	CurrentUsageGB   float64 `json:"current_usage_gb"`
	EstimatedTotalGB float64 `json:"estimated_total_gb"`
	DaysRemaining    float64 `json:"days_remaining"`
}

func handleStorageEstimate(db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var cameras []struct {
			ID            string `db:"id"`
			Name          string `db:"name"`
			RetentionDays int    `db:"retention_days"`
		}
		if err := db.Select(&cameras, "SELECT id, name, retention_days FROM cameras"); err != nil {
			jsonError(w, "failed to fetch cameras", http.StatusInternalServerError)
			return
		}

		var estimates []StorageEstimate
		totalDaily := 0.0
		totalCurrent := 0.0

		for _, cam := range cameras {
			var dailyGB sql.NullFloat64
			db.Get(&dailyGB,
				`SELECT COALESCE(SUM(file_size) / NULLIF(EXTRACT(DAY FROM NOW() - MIN(start_time)), 0), 0) / 1073741824.0
				 FROM recordings WHERE camera_id=$1 AND start_time > NOW() - INTERVAL '7 days'`,
				cam.ID)

			var currentGB sql.NullFloat64
			db.Get(&currentGB,
				`SELECT COALESCE(SUM(file_size), 0) / 1073741824.0 FROM recordings WHERE camera_id=$1`,
				cam.ID)

			daily := dailyGB.Float64
			if daily <= 0 {
				// No data yet: estimate based on default bitrate (4 Mbps)
				// 4 Mbps / 8 * 86400 s/day / 1073741824 bytes/GB ≈ 40 GB/day
				daily = 40.0
			}

			current := currentGB.Float64
			estimated := daily * float64(cam.RetentionDays)
			daysRemaining := float64(cam.RetentionDays) - (current / daily)
			if daysRemaining < 0 {
				daysRemaining = 0
			}

			estimates = append(estimates, StorageEstimate{
				CameraID:         cam.ID,
				CameraName:       cam.Name,
				RetentionDays:    cam.RetentionDays,
				DailyUsageGB:     daily,
				CurrentUsageGB:   current,
				EstimatedTotalGB: estimated,
				DaysRemaining:    daysRemaining,
			})
			totalDaily += daily
			totalCurrent += current
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"estimates":        estimates,
			"total_daily_gb":   totalDaily,
			"total_storage_gb": totalCurrent,
		})
	}
}
