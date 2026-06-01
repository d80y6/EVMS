package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/nats-io/nats.go"
)

type BlurRequest struct {
	CameraID      string       `json:"camera_id"`
	RecordingPath string       `json:"recording_path"`
	Regions       []BlurRegion `json:"regions"`
}

type BlurRegion struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type BlurResult struct {
	CameraID string `json:"camera_id"`
	Success  bool   `json:"success"`
	Path     string `json:"path"`
	Error    string `json:"error,omitempty"`
}

type BlurWorker struct {
	nc     *nats.Conn
	sub    *nats.Subscription
	logger *slog.Logger
}

func NewBlurWorker(nc *nats.Conn) *BlurWorker {
	return &BlurWorker{
		nc:     nc,
		logger: slog.Default().With("component", "blur_worker"),
	}
}

func (w *BlurWorker) Start() error {
	var err error
	w.sub, err = w.nc.QueueSubscribe("blur.request", "blur", w.handleRequest)
	if err != nil {
		return fmt.Errorf("failed to subscribe to blur.request: %w", err)
	}
	w.logger.Info("Blur worker subscribed to blur.request")
	return nil
}

func (w *BlurWorker) Close() error {
	if w.sub != nil {
		return w.sub.Unsubscribe()
	}
	return nil
}

func (w *BlurWorker) handleRequest(msg *nats.Msg) {
	var req BlurRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		w.logger.Error("Failed to unmarshal blur request", "error", err)
		w.publishResult(BlurResult{Success: false, Error: "invalid request"})
		return
	}

	w.logger.Info("Processing blur request", "camera_id", req.CameraID, "path", req.RecordingPath)

	if err := w.processBlur(req); err != nil {
		w.logger.Error("Blur processing failed", "error", err)
		w.publishResult(BlurResult{
			CameraID: req.CameraID,
			Success:  false,
			Path:     req.RecordingPath,
			Error:    err.Error(),
		})
		return
	}

	w.publishResult(BlurResult{
		CameraID: req.CameraID,
		Success:  true,
		Path:     req.RecordingPath,
	})
}

func (w *BlurWorker) processBlur(req BlurRequest) error {
	if _, err := os.Stat(req.RecordingPath); os.IsNotExist(err) {
		return fmt.Errorf("recording file not found: %s", req.RecordingPath)
	}

	ext := filepath.Ext(req.RecordingPath)
	base := req.RecordingPath[:len(req.RecordingPath)-len(ext)]
	outputPath := base + ".blurred" + ext

	var filters []string
	for _, r := range req.Regions {
		filters = append(filters, fmt.Sprintf("drawbox=x=%d:y=%d:w=%d:h=%d:color=black@0.5:t=fill", r.X, r.Y, r.W, r.H))
	}

	args := []string{"-y", "-i", req.RecordingPath, "-vf", strings.Join(filters, ","), "-c:a", "copy", outputPath}

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg failed: %w, output: %s", err, string(output))
	}

	if err := os.Rename(outputPath, req.RecordingPath); err != nil {
		return fmt.Errorf("failed to replace original with blurred: %w", err)
	}

	w.logger.Info("Blur processing complete", "camera_id", req.CameraID, "path", req.RecordingPath)
	return nil
}

func (w *BlurWorker) publishResult(result BlurResult) {
	data, err := json.Marshal(result)
	if err != nil {
		w.logger.Error("Failed to marshal blur result", "error", err)
		return
	}
	if err := w.nc.Publish("blur.completed", data); err != nil {
		w.logger.Error("Failed to publish blur result", "error", err)
	}
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	common.StartMetricsServer(common.GetEnv("METRICS_ADDR", ":2112"))

	natsURL := common.GetEnv("NATS_URL", "nats://localhost:4222")
	nc, err := nats.Connect(natsURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		logger.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()

	worker := NewBlurWorker(nc)
	if err := worker.Start(); err != nil {
		logger.Error("Failed to start blur worker", "error", err)
		os.Exit(1)
	}
	defer worker.Close()

	<-ctx.Done()
	logger.Info("Blur worker shutting down...")
}
