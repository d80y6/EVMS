package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

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
	os.MkdirAll(recordingsPath, 0755)

	args := []string{
		"-rtsp_transport", "tcp",
		"-i", p.URL,
		"-map", "0:v",
		"-c:v:0", "copy",
		"-f", "segment",
		"-segment_time", "60",
		"-segment_format", "mp4",
		"-reset_timestamps", "1",
		filepath.Join(recordingsPath, "%Y-%m-%d_%H-%M-%S.mp4"),
		"-map", "0:v",
		"-s", "640x360",
		"-f", "mjpeg",
		"-pix_fmt", "yuvj420p",
		"pipe:3",
		"-map", "0:v",
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
	go p.publishStream(h264R, fmt.Sprintf("camera.%s.h264", p.CameraID))

	go p.watchRecordings(ctx, recordingsPath)

	slog.Info("Starting FFmpeg process", "camera_id", p.CameraID)
	return p.cmd.Start()
}

func (p *StreamProcessor) watchRecordings(ctx context.Context, path string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer watcher.Close()
	watcher.Add(path)

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-watcher.Events:
			// In production, we'd use IN_CLOSE_WRITE for Linux or a similar robust mechanism.
			// fsnotify on Linux supports CloseWrite.
			if (event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write) && filepath.Ext(event.Name) == ".mp4" {
				// Signal recorder only when file is created/modified.
				if p.nc != nil {
					eventData := map[string]string{
						"camera_id": p.CameraID,
						"path":      event.Name,
					}
					data, _ := json.Marshal(eventData)
					p.nc.Publish(fmt.Sprintf("camera.%s.recordings.new", p.CameraID), data)
				}
			}
		}
	}
}

func (p *StreamProcessor) publishMJPEG(r io.Reader, subject string) {
	scanner := bufio.NewScanner(r)
	// Increase buffer size for high-res JPEG frames
	scanner.Buffer(make([]byte, 0, 1024*1024), 5*1024*1024)

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
			p.nc.Publish(subject, scanner.Bytes())
		}
	}
}

func (p *StreamProcessor) publishStream(r io.Reader, subject string) {
	buf := make([]byte, 1024*1024)
	for {
		n, err := r.Read(buf)
		if err != nil {
			return
		}
		if p.nc != nil {
			// For H264, this is still a bit naive but better than nothing for now.
			// Ideally we would parse NAL units.
			p.nc.Publish(subject, buf[:n])
		}
	}
}

func (p *StreamProcessor) Wait() error {
	return p.cmd.Wait()
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cameraID := os.Getenv("CAMERA_ID")
	rtspURL := os.Getenv("RTSP_URL")
	natsURL := os.Getenv("NATS_URL")

	var nc *nats.Conn
	var err error
	if natsURL != "" {
		nc, err = nats.Connect(natsURL)
		if err != nil {
			slog.Error("Failed to connect to NATS", "error", err)
		} else {
			defer nc.Close()
			slog.Info("Connected to NATS", "url", natsURL)
		}
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
