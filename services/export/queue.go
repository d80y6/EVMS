package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/nats-io/nats.go"
)

type ExportJob struct {
	ID          string     `db:"id" json:"id"`
	CameraID    string     `db:"camera_id" json:"camera_id"`
	StartTime   time.Time  `db:"start_time" json:"start_time"`
	EndTime     time.Time  `db:"end_time" json:"end_time"`
	Watermark   bool       `db:"watermark" json:"watermark"`
	Status      string     `db:"status" json:"status"`
	FilePath    *string    `db:"file_path" json:"file_path,omitempty"`
	SHA256      *string    `db:"sha256" json:"sha256,omitempty"`
	SizeBytes   *int64     `db:"size_bytes" json:"size_bytes,omitempty"`
	Error       *string    `db:"error" json:"error,omitempty"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	CompletedAt *time.Time `db:"completed_at" json:"completed_at,omitempty"`
}

type ExportJobProducer struct {
	db     *sqlx.DB
	nc     *nats.Conn
	logger *slog.Logger
}

type ExportJobConsumer struct {
	db     *sqlx.DB
	logger *slog.Logger
}

func NewExportJobProducer(db *sqlx.DB, nc *nats.Conn, logger *slog.Logger) *ExportJobProducer {
	return &ExportJobProducer{db: db, nc: nc, logger: logger}
}

func NewExportJobConsumer(db *sqlx.DB, logger *slog.Logger) *ExportJobConsumer {
	return &ExportJobConsumer{db: db, logger: logger}
}

func (p *ExportJobProducer) CreateJob(ctx context.Context, req ExportRequest) (*ExportJob, error) {
	id := uuid.New().String()
	now := time.Now()

	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, fmt.Errorf("invalid start_time: %w", err)
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, fmt.Errorf("invalid end_time: %w", err)
	}

	job := &ExportJob{
		ID:        id,
		CameraID:  req.CameraID,
		StartTime: startTime,
		EndTime:   endTime,
		Watermark: req.Watermark,
		Status:    "queued",
		CreatedAt: now,
	}

	_, err = p.db.ExecContext(ctx,
		`INSERT INTO export_jobs (id, camera_id, start_time, end_time, watermark, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, 'queued', $6)`,
		id, req.CameraID, startTime, endTime, req.Watermark, now)
	if err != nil {
		return nil, fmt.Errorf("failed to create export job: %w", err)
	}

	data, _ := json.Marshal(job)
	if err := p.nc.Publish("export.jobs", data); err != nil {
		p.logger.Error("Failed to publish export job", "job_id", id, "error", err)
	}

	return job, nil
}

func (c *ExportJobConsumer) RecoverStuckJobs(ctx context.Context) {
	result, err := c.db.ExecContext(ctx,
		`UPDATE export_jobs SET status = 'queued', updated_at = NOW()
		 WHERE status = 'processing' AND updated_at < NOW() - INTERVAL '30 minutes'`)
	if err != nil {
		c.logger.Error("Failed to recover stuck jobs", "error", err)
		return
	}
	if n, _ := result.RowsAffected(); n > 0 {
		c.logger.Info("Recovered stuck export jobs", "count", n)
	}
}

func (c *ExportJobConsumer) ProcessJob(ctx context.Context, job *ExportJob) {
	c.logger.Info("Processing export job", "job_id", job.ID, "camera_id", job.CameraID)

	if _, err := c.db.ExecContext(ctx, "UPDATE export_jobs SET status = 'processing', updated_at = NOW() WHERE id = $1", job.ID); err != nil {
		c.logger.Error("Failed to update job status to processing", "job_id", job.ID, "error", err)
		return
	}

	startStr := job.StartTime.Format(time.RFC3339)
	endStr := job.EndTime.Format(time.RFC3339)
	segments, err := findSegments(job.CameraID, startStr, endStr)
	if err != nil {
		c.failJob(ctx, job.ID, fmt.Sprintf("failed to find segments: %v", err))
		return
	}
	if len(segments) == 0 {
		c.failJob(ctx, job.ID, "no recordings found")
		return
	}

	cameraID := sanitizeCameraID(job.CameraID)
	outputPath := filepath.Join("/exports", fmt.Sprintf("export_%s_%s.mp4", cameraID, job.ID[:8]))

	args := []string{"-y"}
	for _, seg := range segments {
		if err := common.ValidateRecordingPath(seg); err != nil {
			c.failJob(ctx, job.ID, fmt.Sprintf("invalid segment path: %s", seg))
			return
		}
		if err := common.ValidateFilePath(seg, "/recordings"); err != nil {
			c.failJob(ctx, job.ID, fmt.Sprintf("segment path outside allowed root: %s", seg))
			return
		}
		args = append(args, "-i", seg)
	}
	filter := fmt.Sprintf("concat=%d", len(segments))
	if job.Watermark {
		content := fmt.Sprintf("%%{localtime} | Camera: %s", cameraID)
		tmpFile, err := os.CreateTemp("", "watermark_*.txt")
		if err != nil {
			c.failJob(ctx, job.ID, "failed to create watermark file")
			return
		}
		watermarkPath := tmpFile.Name()
		if _, err := tmpFile.WriteString(content); err != nil {
			tmpFile.Close()
			os.Remove(watermarkPath)
			c.failJob(ctx, job.ID, "failed to write watermark file")
			return
		}
		tmpFile.Close()
		defer os.Remove(watermarkPath)
		filter += fmt.Sprintf(",drawtext=textfile='%s':fontsize=24:fontcolor=white:x=10:y=10", watermarkPath)
	}
	args = append(args, "-filter_complex", filter, "-c:v", "libx264", "-preset", "fast", outputPath)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		c.failJob(ctx, job.ID, fmt.Sprintf("ffmpeg failed: %v - %s", err, stderr.String()))
		return
	}

	f, err := os.Open(outputPath)
	if err != nil {
		c.failJob(ctx, job.ID, fmt.Sprintf("failed to read export: %v", err))
		return
	}
	defer f.Close()
	h := sha256.New()
	size, _ := io.Copy(h, f)
	checksum := fmt.Sprintf("%x", h.Sum(nil))
	sizeBytes := size

	now := time.Now()
	if _, err := c.db.ExecContext(ctx,
		`UPDATE export_jobs SET status = 'completed', file_path = $1, sha256 = $2, size_bytes = $3, completed_at = $4, updated_at = $4 WHERE id = $5`,
		outputPath, checksum, sizeBytes, now, job.ID); err != nil {
		c.logger.Error("Failed to update export job as completed", "job_id", job.ID, "error", err)
	}
	c.logger.Info("Export job completed", "job_id", job.ID, "file", outputPath, "sha256", checksum)
}

func (c *ExportJobConsumer) failJob(ctx context.Context, jobID, errMsg string) {
	c.logger.Error("Export job failed", "job_id", jobID, "error", errMsg)
	now := time.Now()
	if _, err := c.db.ExecContext(ctx,
		`UPDATE export_jobs SET status = 'failed', error = $1, completed_at = $2, updated_at = $2 WHERE id = $3`,
		errMsg, now, jobID); err != nil {
		c.logger.Error("Failed to update failed job status", "job_id", jobID, "error", err)
	}
}
