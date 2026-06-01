package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/dam-vms/dam/pkg/common"
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

func findSegments(cameraID, start, end string) ([]string, error) {
	dir := fmt.Sprintf("/recordings/%s", cameraID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var segments []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".mp4" {
			segments = append(segments, filepath.Join(dir, e.Name()))
		}
	}
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

	outputPath := filepath.Join("/exports", fmt.Sprintf("export_%s_%s.mp4", req.CameraID, time.Now().Format("20060102150405")))
	args := []string{"-y"}
	for _, seg := range segments {
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

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	common.StartMetricsServer(common.GetEnv("METRICS_ADDR", ":2112"))

	mux := http.NewServeMux()
	mux.HandleFunc("/export", handleExport)

	server := &http.Server{
		Addr:         ":8094",
		Handler:      mux,
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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}
