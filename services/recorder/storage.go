package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
)

type ingestionSample struct {
	bytes     int64
	timestamp time.Time
}

type IngestionRateTracker struct {
	mu       sync.RWMutex
	rates    map[string][]ingestionSample
	window   time.Duration
}

func NewIngestionRateTracker(window time.Duration) *IngestionRateTracker {
	return &IngestionRateTracker{
		rates:  make(map[string][]ingestionSample),
		window: window,
	}
}

func (t *IngestionRateTracker) Record(cameraID string, bytes int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.rates[cameraID] = append(t.rates[cameraID], ingestionSample{bytes: bytes, timestamp: now})
	cutoff := now.Add(-t.window)
	samples := t.rates[cameraID]
	keep := len(samples)
	for i, s := range samples {
		if s.timestamp.After(cutoff) {
			keep = i
			break
		}
	}
	if keep > 0 {
		t.rates[cameraID] = samples[keep:]
	}
}

func (t *IngestionRateTracker) GetRate(cameraID string) float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	samples := t.rates[cameraID]
	if len(samples) < 2 {
		return 0
	}
	var totalBytes int64
	for _, s := range samples {
		totalBytes += s.bytes
	}
	duration := samples[len(samples)-1].timestamp.Sub(samples[0].timestamp).Seconds()
	if duration <= 0 {
		return 0
	}
	return float64(totalBytes) / duration
}

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

type StorageForecast struct {
	CameraID      string  `json:"camera_id"`
	CameraName    string  `json:"camera_name"`
	CurrentUsageGB float64 `json:"current_usage_gb"`
	DailyRateGB   float64 `json:"daily_rate_gb"`
	RetentionDays int     `json:"retention_days"`
	Forecast7dGB  float64 `json:"forecast_7d_gb"`
	Forecast30dGB float64 `json:"forecast_30d_gb"`
	DaysRemaining float64 `json:"days_remaining"`
}

func handleStorageForecast(db *sqlx.DB, tracker *IngestionRateTracker) http.HandlerFunc {
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

		var forecasts []StorageForecast
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
			rate := tracker.GetRate(cam.ID)
			if daily <= 0 && rate > 0 {
				daily = rate * 86400 / 1073741824.0
			}
			if daily <= 0 {
				daily = 40.0
			}

			current := currentGB.Float64
			forecast7d := current + daily*7
			forecast30d := current + daily*30
			daysRemaining := float64(cam.RetentionDays) - (current / daily)
			if daysRemaining < 0 {
				daysRemaining = 0
			}

			forecasts = append(forecasts, StorageForecast{
				CameraID:       cam.ID,
				CameraName:     cam.Name,
				CurrentUsageGB: current,
				DailyRateGB:    daily,
				RetentionDays:  cam.RetentionDays,
				Forecast7dGB:   forecast7d,
				Forecast30dGB:  forecast30d,
				DaysRemaining:  daysRemaining,
			})
			totalDaily += daily
			totalCurrent += current
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"forecasts":        forecasts,
			"total_daily_gb":   totalDaily,
			"total_storage_gb": totalCurrent,
		})
	}
}
