package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type RecordingSegment struct {
	ID         string    `db:"id"`
	CameraID   string    `db:"camera_id"`
	StartTime  time.Time `db:"start_time"`
	EndTime    time.Time `db:"end_time"`
	FilePath   string    `db:"file_path"`
	FileSize   int64     `db:"file_size"`
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

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		dbURL = "postgres://dam_admin:dam_password@localhost:5432/dam_vms?sslmode=disable"
	}

	db, err := sqlx.Connect("postgres", dbURL)
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	_ = &Recorder{db: db, logger: logger}
	fmt.Println("Recorder service started")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down recorder...")
}
