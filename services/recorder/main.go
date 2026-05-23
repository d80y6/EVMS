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
	r.logger.Info("Indexed recording segment", "camera_id", seg.CameraID, "path", seg.FilePath)
	return nil
}

func (r *Recorder) Listen(ctx context.Context, nc *nats.Conn) error {
	_, err := nc.Subscribe("camera.*.recordings.new", func(msg *nats.Msg) {
		var event struct {
			CameraID string `json:"camera_id"`
			Path     string `json:"path"`
		}
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			return
		}

		// Wait briefly for file to finalize if necessary, though NATS is better than pure polling
		time.Sleep(1 * time.Second)

		info, err := os.Stat(event.Path)
		if err != nil {
			return
		}

		filename := filepath.Base(event.Path)
		timeStr := strings.TrimSuffix(filename, ".mp4")
		startTime, err := time.Parse("2006-01-02_15-04-05", timeStr)
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

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	dbURL := os.Getenv("DB_URL")
	natsURL := os.Getenv("NATS_URL")

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

	<-ctx.Done()
	logger.Info("Shutting down recorder...")
}
