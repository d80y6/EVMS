package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/jmoiron/sqlx"
	"github.com/nats-io/nats.go"
	_ "github.com/lib/pq"
)

var exportProducer *ExportJobProducer

type ExportRequest struct {
	CameraID    string `json:"camera_id"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	Watermark   bool   `json:"watermark"`
	RequestedBy string `json:"requested_by"`
}

type ExportResult struct {
	FilePath string `json:"file_path"`
	Checksum string `json:"sha256"`
	Size     int64  `json:"size_bytes"`
}

func sanitizeCameraID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || id == "." || id == ".." {
		return ""
	}
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return -1
	}, id)
}

func findSegments(cameraID, start, end string) ([]string, error) {
	cameraID = sanitizeCameraID(cameraID)
	if cameraID == "" {
		return nil, fmt.Errorf("invalid camera_id")
	}
	dir := fmt.Sprintf("/recordings/%s", cameraID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	startTime, startErr := time.Parse(time.RFC3339, start)
	endTime, endErr := time.Parse(time.RFC3339, end)
	var segments []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".mp4" {
			continue
		}
		if startErr == nil || endErr == nil {
			// Filenames from FFmpeg -strftime: 20240608_120000.mp4
			segTime, segErr := time.Parse("20060102_150405", strings.TrimSuffix(e.Name(), ".mp4"))
			if segErr == nil {
				if startErr == nil && segTime.Before(startTime) {
					continue
				}
				if endErr == nil && segTime.After(endTime) {
					continue
				}
			}
		}
		segments = append(segments, filepath.Join(dir, e.Name()))
	}
	sort.Strings(segments)
	return segments, nil
}

func handleExport(w http.ResponseWriter, r *http.Request) {
	var req ExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}

	if exportProducer == nil {
		jsonError(w, "export queue not available", http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()
	job, err := exportProducer.CreateJob(ctx, req)
	if err != nil {
		slog.Error("Failed to create export job", "error", err)
		jsonError(w, "failed to create export job", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusAccepted, job)
}

type jobStatusResponse struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	FilePath    *string    `json:"file_path,omitempty"`
	SHA256      *string    `json:"sha256,omitempty"`
	SizeBytes   *int64     `json:"size_bytes,omitempty"`
	Error       *string    `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func handleExportStatus(db *sqlx.DB, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			jsonError(w, "database not configured", http.StatusInternalServerError)
			return
		}
		jobID := strings.TrimPrefix(r.URL.Path, "/export/status/")
		if jobID == "" {
			jsonError(w, "job id required", http.StatusBadRequest)
			return
		}

		var job jobStatusResponse
		err := db.GetContext(r.Context(), &job,
			"SELECT id, status, file_path, sha256, size_bytes, error, created_at, completed_at FROM export_jobs WHERE id = $1",
			jobID)
		if err != nil {
			jsonError(w, "job not found", http.StatusNotFound)
			return
		}

		writeJSON(w, http.StatusOK, job)
	}
}

func handleExportDownload(db *sqlx.DB, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			jsonError(w, "database not configured", http.StatusInternalServerError)
			return
		}
		jobID := strings.TrimPrefix(r.URL.Path, "/export/download/")
		if jobID == "" {
			jsonError(w, "job id required", http.StatusBadRequest)
			return
		}

		var job struct {
			Status   string  `db:"status"`
			FilePath *string `db:"file_path"`
		}
		err := db.GetContext(r.Context(), &job,
			"SELECT status, file_path FROM export_jobs WHERE id = $1", jobID)
		if err != nil {
			jsonError(w, "job not found", http.StatusNotFound)
			return
		}

		if job.Status != "completed" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "export not yet completed", "status": job.Status})
			return
		}

		if job.FilePath == nil {
			jsonError(w, "file path not available", http.StatusInternalServerError)
			return
		}

		http.ServeFile(w, r, *job.FilePath)
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func main() {
	logger := common.NewLogger("export")
	slog.SetDefault(logger)

	common.CheckJWTSecret()

	if err := common.InitTelemetry("export"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	common.StartMetricsServer(common.GetEnv("METRICS_ADDR", ":2112"))
	common.StartResourceMonitor(ctx)

	dbURL := os.Getenv("DB_URL")
	var db *sqlx.DB
	if dbURL != "" {
		cb := common.NewDBCircuitBreaker("export")
		dbCtx, dbCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer dbCancel()
		var err error
		db, err = common.ConnectDBWithCircuitBreaker(dbCtx, "postgres", dbURL, cb)
		if err != nil {
			logger.Error("Failed to connect to database", "error", err)
		} else {
			logger.Info("Connected to database")
		}
	}

	natsURL := os.Getenv("NATS_URL")
	var nc *nats.Conn
	if natsURL != "" && db != nil {
		natsCB := common.NewNATSCircuitBreaker("export")
		var err error
		nc, err = common.ConnectNATSWithCircuitBreaker(natsURL, natsCB)
		if err != nil {
			logger.Error("Failed to connect to NATS", "error", err)
		} else {
			logger.Info("Connected to NATS")
			exportProducer = NewExportJobProducer(db, nc, logger)
			consumer := NewExportJobConsumer(db, logger)

			// Recover jobs stuck in processing or queued from prior crashes
			consumer.RecoverStuckJobs(context.Background())

			sub, err := nc.QueueSubscribe("export.jobs", "export", func(msg *nats.Msg) {
				var job ExportJob
				if err := json.Unmarshal(msg.Data, &job); err != nil {
					logger.Error("Failed to unmarshal export job", "error", err)
					return
				}
				go consumer.ProcessJob(context.Background(), &job)
			})
			if err != nil {
				logger.Error("Failed to subscribe to export.jobs", "error", err)
				sub = nil
			} else {
				logger.Info("Export worker subscribed to export.jobs")
			}

		if sub != nil {
			defer sub.Unsubscribe()
		}
		}
	}

	mux := http.NewServeMux()
	healthHandler := common.NewHealthHandler()
	if db != nil {
		healthHandler.AddDBChecker(db.DB, "postgres")
	}
	if nc != nil {
		healthHandler.AddNATSChecker(nc, "nats")
	}
	mux.HandleFunc("/health", healthHandler.Liveness)
	mux.HandleFunc("/ready", healthHandler.Readiness)
	mux.Handle("/export", common.JWTAuthMiddleware(handleExport))
	mux.Handle("/export/status/", common.JWTAuthMiddleware(handleExportStatus(db, logger)))
	mux.Handle("/export/download/", common.JWTAuthMiddleware(handleExportDownload(db, logger)))

	if db != nil {
		mux.Handle("/api/evidence/cases", common.JWTAuthMiddleware(handleEvidenceCases(db, logger)))
		mux.Handle("/api/evidence/cases/", common.JWTAuthMiddleware(handleEvidenceCaseByID(db, logger)))
		mux.Handle("/api/evidence/lockers", common.JWTAuthMiddleware(handleEvidenceLockers(db, logger)))
		mux.Handle("/api/evidence/lockers/", common.JWTAuthMiddleware(handleEvidenceLockerByID(db, logger)))
		mux.Handle("/api/evidence/items", common.JWTAuthMiddleware(handleEvidenceItems(db, logger)))
		mux.Handle("/api/evidence/items/", common.JWTAuthMiddleware(handleEvidenceItemByID(db, logger)))
		mux.Handle("/api/evidence/share/", common.JWTAuthMiddleware(handleShareAccess(db, logger)))
	}

	server := &http.Server{
		Addr:         ":8094",
		Handler:      common.RecoveryMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("Export service listening", "addr", ":8094")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
		}
	}()

	<-ctx.Done()
	if nc != nil {
		nc.Drain()
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}
