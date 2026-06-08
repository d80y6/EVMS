package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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

	segments, err := findSegments(req.CameraID, req.StartTime, req.EndTime)
	if err != nil {
		jsonError(w, "failed to find segments", http.StatusInternalServerError)
		return
	}
	if len(segments) == 0 {
		jsonError(w, "no recordings found", http.StatusNotFound)
		return
	}

	req.CameraID = sanitizeCameraID(req.CameraID)
	outputPath := filepath.Join("/exports", fmt.Sprintf("export_%s_%s.mp4", req.CameraID, time.Now().Format("20060102150405")))
	args := []string{"-y"}
	for _, seg := range segments {
		if err := common.ValidateRecordingPath(seg); err != nil {
			jsonError(w, fmt.Sprintf("invalid segment path: %s", seg), http.StatusBadRequest)
			return
		}
		if err := common.ValidateFilePath(seg, "/recordings"); err != nil {
			jsonError(w, fmt.Sprintf("segment path outside allowed root: %s", seg), http.StatusBadRequest)
			return
		}
		args = append(args, "-i", seg)
	}
	filter := fmt.Sprintf("concat=%d", len(segments))
	if req.Watermark {
		filter += ",drawtext=text='%{localtime} | Camera: " + req.CameraID + "':fontsize=24:fontcolor=white:x=10:y=10"
	}
	args = append(args, "-filter_complex", filter, "-c:v", "libx264", "-preset", "fast", outputPath)

	cmd := exec.CommandContext(r.Context(), "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		slog.Error("ffmpeg export failed", "error", err, "stderr", stderr.String())
		jsonError(w, "export failed", http.StatusInternalServerError)
		return
	}

	f, err := os.Open(outputPath)
	if err != nil {
		jsonError(w, "failed to read export", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	h := sha256.New()
	size, _ := io.Copy(h, f)
	checksum := fmt.Sprintf("%x", h.Sum(nil))

	json.NewEncoder(w).Encode(ExportResult{
		FilePath: outputPath,
		Checksum: checksum,
		Size:     size,
	})
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
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

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

	mux := http.NewServeMux()
	healthHandler := common.NewHealthHandler()
	if db != nil {
		healthHandler.AddDBChecker(db.DB, "postgres")
	}
	mux.HandleFunc("/health", healthHandler.Liveness)
	mux.HandleFunc("/ready", healthHandler.Readiness)
	mux.Handle("/export", common.JWTAuthMiddleware(handleExport))

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
	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}
