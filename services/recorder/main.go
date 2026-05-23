package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
)

type RecordingSegment struct {
	CameraID  string    `db:"camera_id"`
	StartTime time.Time `db:"start_time"`
	EndTime   time.Time `db:"end_time"`
	FilePath  string    `db:"file_path"`
	FileSize  int64     `db:"file_size"`
}

type Recorder struct {
	db     *sqlx.DB
	logger *slog.Logger
}

func (r *Recorder) IndexSegment(ctx context.Context, seg RecordingSegment) error {
	query := `INSERT INTO recordings (camera_id, start_time, end_time, file_path, file_size)
              VALUES (:camera_id, :start_time, :end_time, :file_path, :file_size)`
	_, err := r.db.NamedExecContext(ctx, query, seg)
	if err != nil {
		r.logger.Error("Failed to index segment", "error", err, "camera_id", seg.CameraID)
		return err
	}
	r.logger.Info("Indexed recording segment", "camera_id", seg.CameraID, "path", seg.FilePath, "size", seg.FileSize)
	common.RecordingsIndexed.WithLabelValues(seg.CameraID).Inc()
	return nil
}

func (r *Recorder) Listen(ctx context.Context, nc *nats.Conn) error {
	// Use JetStream if possible for better durability, but stick to Core NATS for this foundation
	_, err := nc.Subscribe("camera.*.recordings.new", func(msg *nats.Msg) {
		var event struct {
			CameraID string `json:"camera_id"`
			Path     string `json:"path"`
		}
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			return
		}

		// Reliability: Wait for file to be finalized by FFmpeg
		// In a production system, we'd use a .tmp -> .mp4 rename or a 'close' event
		maxRetries := 5
		var info os.FileInfo
		var err error
		for i := 0; i < maxRetries; i++ {
			info, err = os.Stat(event.Path)
			if err == nil && info.Size() > 0 {
				break
			}
			time.Sleep(2 * time.Second)
		}

		if err != nil {
			r.logger.Warn("Could not stat recording file", "path", event.Path, "error", err)
			return
		}

		filename := filepath.Base(event.Path)
		timeStr := strings.TrimSuffix(filename, ".mp4")
		// Format from ingest: 20240101_120000.mp4
		startTime, err := time.Parse("20060102_150405", timeStr)
		if err != nil {
			startTime = time.Now()
		}

		seg := RecordingSegment{
			CameraID:  event.CameraID,
			StartTime: startTime,
			EndTime:   startTime.Add(60 * time.Second),
			FilePath:  event.Path,
			FileSize:  info.Size(),
		}

		r.IndexSegment(ctx, seg)
	})
	return err
}

func (r *Recorder) StartRetentionWorker(ctx context.Context, interval time.Duration, retentionDays int) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runRetentionCleanup(ctx, retentionDays)
		}
	}
}

func (r *Recorder) runRetentionCleanup(ctx context.Context, retentionDays int) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	r.logger.Info("Running retention cleanup", "cutoff", cutoff)

	var segments []RecordingSegment
	err := r.db.SelectContext(ctx, &segments, "SELECT camera_id, file_path FROM recordings WHERE start_time < $1", cutoff)
	if err != nil {
		r.logger.Error("Failed to fetch expired segments", "error", err)
		return
	}

	for _, seg := range segments {
		if err := os.Remove(seg.FilePath); err != nil && !os.IsNotExist(err) {
			r.logger.Error("Failed to delete recording file", "path", seg.FilePath, "error", err)
		} else {
			_, err = r.db.ExecContext(ctx, "DELETE FROM recordings WHERE file_path = $1", seg.FilePath)
			if err != nil {
				r.logger.Error("Failed to delete recording record", "path", seg.FilePath, "error", err)
			}
		}
	}
	r.logger.Info("Retention cleanup finished", "deleted_count", len(segments))
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	common.StartMetricsServer(":2112")

	dbURL := os.Getenv("DB_URL")
	natsURL := os.Getenv("NATS_URL")
	retentionDays := 7 // Default 7 days

	db, err := sqlx.Connect("postgres", dbURL)
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	nc, err := nats.Connect(natsURL)
	if err != nil {
		logger.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()

	recorder := &Recorder{db: db, logger: logger}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("Recorder service listening for NATS signals...")
	if err := recorder.Listen(ctx, nc); err != nil {
		logger.Error("Failed to start listener", "error", err)
		os.Exit(1)
	}

	// Start background retention worker
	go recorder.StartRetentionWorker(ctx, 1*time.Hour, retentionDays)

	<-ctx.Done()
	logger.Info("Shutting down recorder...")
}
