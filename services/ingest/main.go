package main

import (
	"bufio"
	"bytes"
	"context"
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
	"github.com/dam-vms/dam/services/ingest/internal/health"
	"github.com/fsnotify/fsnotify"
	"github.com/nats-io/nats.go"
)

type StreamProcessor struct {
	CameraID string
	URL      string
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	nc       *nats.Conn
}

func NewStreamProcessor(cameraID, url string, nc *nats.Conn) *StreamProcessor {
	return &StreamProcessor{
		CameraID: cameraID,
		URL:      url,
		nc:       nc,
	}
}

func (p *StreamProcessor) Start(ctx context.Context) error {
	childCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	recordingsPath := fmt.Sprintf("/recordings/%s", p.CameraID)
	if err := os.MkdirAll(recordingsPath, 0755); err != nil {
		return fmt.Errorf("failed to create recordings directory: %w", err)
	}

	// Optimized FFmpeg args for low latency and consistent segmenting
	args := []string{
		"-rtsp_transport", "tcp",
		"-hwaccel", "auto", // Enable hardware acceleration if available
		"-i", p.URL,
		// Stream 0: Video copy for recording
		"-map", "0:v:0",
		"-c:v:0", "copy",
		"-f", "segment",
		"-segment_time", "60",
		"-segment_format", "mp4",
		"-segment_atclocktime", "1",
		"-reset_timestamps", "1",
		"-strftime", "1",
		filepath.Join(recordingsPath, "%Y%m%d_%H%M%S.mp4"),
		// Stream 1: Low-res MJPEG for AI/Preview
		"-map", "0:v:0",
		"-s", "640x360",
		"-f", "mjpeg",
		"-q:v", "5",
		"-fpsmax", "10",
		"pipe:3",
		// Stream 2: H264 for WebRTC
		"-map", "0:v:0",
		"-c:v:1", "copy",
		"-f", "h264",
		"-bsf:v", "h264_mp4toannexb",
		"pipe:4",
	}

	p.cmd = exec.CommandContext(childCtx, "ffmpeg", args...)

	mjpegR, mjpegW, _ := os.Pipe()
	h264R, h264W, _ := os.Pipe()
	p.cmd.ExtraFiles = []*os.File{mjpegW, h264W}

	go p.publishMJPEG(mjpegR, fmt.Sprintf("camera.%s.frames", p.CameraID))
	go p.publishH264(h264R, fmt.Sprintf("camera.%s.h264", p.CameraID))

	go p.watchRecordings(childCtx, recordingsPath)

	common.StreamActive.WithLabelValues(p.CameraID).Set(1)

	slog.Info("Starting FFmpeg process", "camera_id", p.CameraID, "url", p.URL)

	if err := p.cmd.Start(); err != nil {
		cancel()
		return err
	}
	return nil
}

func (p *StreamProcessor) watchRecordings(ctx context.Context, path string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("Failed to create fsnotify watcher", "error", err)
		return
	}
	defer watcher.Close()
	watcher.Add(path)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// Use IN_CLOSE_WRITE equivalent (OpWrite or OpCreate then OpWrite)
			// For simplicity in this demo, we trigger on any mp4 creation/update.
			// In production, we'd wait for a "finalized" signal or use a .tmp extension.
			if event.Has(fsnotify.Create) && filepath.Ext(event.Name) == ".mp4" {
				if p.nc != nil {
					eventData := map[string]string{
						"camera_id": p.CameraID,
						"path":      event.Name,
					}
					data, _ := json.Marshal(eventData)
					// Publish with retry or JetStream for reliability
					p.nc.Publish(fmt.Sprintf("camera.%s.recordings.new", p.CameraID), data)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			slog.Error("Watcher error", "error", err)
		}
	}
}

func (p *StreamProcessor) publishMJPEG(r io.Reader, subject string) {
	defer func() {
		if rc, ok := r.(io.ReadCloser); ok {
			rc.Close()
		}
	}()

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	// Split by JPEG SOI/EOI
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		if i := bytes.Index(data, []byte{0xff, 0xd8}); i >= 0 {
			if j := bytes.Index(data[i+2:], []byte{0xff, 0xd9}); j >= 0 {
				return i + j + 4, data[i : i+j+4], nil
			}
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	})

	for scanner.Scan() {
		if p.nc != nil {
			if err := p.nc.Publish(subject, scanner.Bytes()); err != nil {
				slog.Error("Failed to publish MJPEG", "error", err)
			}
			common.FramesProcessed.WithLabelValues(p.CameraID, "mjpeg").Inc()
		}
	}
}

func (p *StreamProcessor) publishH264(r io.Reader, subject string) {
	defer func() {
		if rc, ok := r.(io.ReadCloser); ok {
			rc.Close()
		}
	}()

	// Robust H264 NAL unit fragmentation for NATS
	// We look for Annex B start codes (0x000001 or 0x00000001)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}

		// Look for next start code
		if i := bytes.Index(data, []byte{0x00, 0x00, 0x01}); i >= 0 {
			// Find subsequent start code to determine NAL unit boundary
			if j := bytes.Index(data[i+3:], []byte{0x00, 0x00, 0x01}); j >= 0 {
				return i + j + 3, data[i : i+j+3], nil
			}
		}

		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	})

	for scanner.Scan() {
		if p.nc != nil {
			p.nc.Publish(subject, scanner.Bytes())
			common.FramesProcessed.WithLabelValues(p.CameraID, "h264").Inc()
		}
	}
}

func (p *StreamProcessor) Wait() error {
	return p.cmd.Wait()
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", health.Handler("ingest"))
		http.ListenAndServe(":8080", mux)
	}()

	common.StartMetricsServer(":2112")

	cameraID := os.Getenv("CAMERA_ID")
	rtspURL := os.Getenv("RTSP_URL")
	natsURL := os.Getenv("NATS_URL")

	if cameraID == "" || rtspURL == "" {
		slog.Error("CAMERA_ID and RTSP_URL must be set")
		os.Exit(1)
	}

	var nc *nats.Conn
	var err error
	if natsURL != "" {
		for i := 0; i < 5; i++ {
			nc, err = nats.Connect(natsURL)
			if err == nil {
				break
			}
			slog.Warn("Retrying NATS connection...", "error", err, "attempt", i+1)
			time.Sleep(2 * time.Second)
		}
		if err != nil {
			slog.Error("Failed to connect to NATS", "error", err)
			os.Exit(1)
		}
		defer nc.Close()
		slog.Info("Connected to NATS", "url", natsURL)
	}

	processor := NewStreamProcessor(cameraID, rtspURL, nc)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := processor.Start(ctx); err != nil {
		slog.Error("Failed to start processor", "error", err)
		os.Exit(1)
	}

	go func() {
		if err := processor.Wait(); err != nil {
			slog.Error("FFmpeg process exited", "error", err)
		}
		stop()
	}()

	<-ctx.Done()
	slog.Info("Shutting down ingest service...")
}
