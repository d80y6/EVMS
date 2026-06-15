package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/jmoiron/sqlx"
)

type GapDetector struct {
	db     *sqlx.DB
	logger *slog.Logger
}

type IntegrityVerifier struct {
	db     *sqlx.DB
	logger *slog.Logger
}

func NewGapDetector(db *sqlx.DB, logger *slog.Logger) *GapDetector {
	return &GapDetector{db: db, logger: logger}
}

func NewIntegrityVerifier(db *sqlx.DB, logger *slog.Logger) *IntegrityVerifier {
	return &IntegrityVerifier{db: db, logger: logger}
}

func (gd *GapDetector) Run(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	gd.logger.Info("Starting gap detector", "interval", "15m")
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			gd.detectGaps(ctx)
		}
	}
}

func (gd *GapDetector) detectGaps(ctx context.Context) {
	var cameras []string
	err := gd.db.SelectContext(ctx, &cameras,
		"SELECT DISTINCT camera_id FROM recordings WHERE start_time > NOW() - INTERVAL '24 hours'")
	if err != nil {
		gd.logger.Error("Gap detector: failed to query cameras", "error", err)
		return
	}

	const maxGap = 65 * time.Second
	for _, camID := range cameras {
		type seg struct {
			StartTime time.Time `db:"start_time"`
			EndTime   time.Time `db:"end_time"`
		}
		var segments []seg
		err := gd.db.SelectContext(ctx, &segments,
			"SELECT start_time, end_time FROM recordings WHERE camera_id = $1 AND start_time > NOW() - INTERVAL '24 hours' ORDER BY start_time",
			camID)
		if err != nil {
			gd.logger.Error("Gap detector: failed to query segments", "camera_id", camID, "error", err)
			continue
		}

		for i := 1; i < len(segments); i++ {
			gap := segments[i].StartTime.Sub(segments[i-1].EndTime)
			if gap > maxGap {
				gd.logger.Error("Recording gap detected",
					"camera_id", camID,
					"expected_start", segments[i-1].EndTime,
					"actual_start", segments[i].StartTime,
					"gap_seconds", int(gap.Seconds()))
				common.RecordingGaps.WithLabelValues(camID).Inc()
			}
		}
	}
}

func computeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file for checksum: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("failed to compute checksum: %w", err)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (iv *IntegrityVerifier) Run(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	iv.logger.Info("Starting integrity verifier", "interval", "24h")
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			iv.verifyIntegrity(ctx)
		}
	}
}

func (iv *IntegrityVerifier) verifyIntegrity(ctx context.Context) {
	type recordingCheck struct {
		ID       string `db:"id"`
		FilePath string `db:"file_path"`
		SHA256   string `db:"sha256"`
	}
	var recordings []recordingCheck
	err := iv.db.SelectContext(ctx, &recordings,
		`SELECT id, file_path, sha256 FROM recordings
         WHERE sha256 IS NOT NULL
           AND (last_verified IS NULL OR last_verified < NOW() - INTERVAL '7 days')
         ORDER BY random()
         LIMIT GREATEST(1, (SELECT COUNT(*) * 0.05 FROM recordings WHERE sha256 IS NOT NULL))`)
	if err != nil {
		iv.logger.Error("Integrity verifier: failed to query recordings", "error", err)
		return
	}

	for _, rec := range recordings {
		actual, err := computeSHA256(rec.FilePath)
		if err != nil {
			iv.logger.Warn("Integrity verifier: failed to compute checksum", "id", rec.ID, "error", err)
			continue
		}
		if actual != rec.SHA256 {
			iv.logger.Error("Recording integrity mismatch",
				"id", rec.ID, "file_path", rec.FilePath,
				"expected", rec.SHA256, "actual", actual)
		}
		iv.db.ExecContext(ctx, "UPDATE recordings SET last_verified = NOW() WHERE id = $1", rec.ID)
	}
}
