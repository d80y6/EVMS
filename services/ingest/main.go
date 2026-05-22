package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

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

	args := []string{
		"-rtsp_transport", "tcp",
		"-i", p.URL,
		"-map", "0:v",
		"-c:v", "copy",
		"-f", "segment",
		"-segment_time", "60",
		"-segment_format", "mp4",
		"-reset_timestamps", "1",
		fmt.Sprintf("/recordings/%s/%%Y-%%m-%%d_%%H-%%M-%%S.mp4", p.CameraID),
		"-map", "0:v",
		"-s", "640x360",
		"-f", "mjpeg",
		"-pix_fmt", "yuvj420p",
		"pipe:1",
	}

	p.cmd = exec.CommandContext(childCtx, "ffmpeg", args...)
	os.MkdirAll(fmt.Sprintf("/recordings/%s", p.CameraID), 0755)

	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		return err
	}

	go p.publishFrames(stdout)

	slog.Info("Starting FFmpeg process", "camera_id", p.CameraID)
	return p.cmd.Start()
}

// publishFrames scans the MJPEG stream for SOI (0xFFD8) and EOI (0xFFD9) markers
func (p *StreamProcessor) publishFrames(r io.Reader) {
	scanner := bufio.NewScanner(r)

	// MJPEG scanner split function
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}

		// Find SOI
		start := bytes.Index(data, []byte{0xFF, 0xD8})
		if start == -1 {
			return len(data), nil, nil
		}

		// Find EOI after SOI
		end := bytes.Index(data[start+2:], []byte{0xFF, 0xD9})
		if end == -1 {
			if atEOF {
				return len(data), data[start:], nil
			}
			return start, nil, nil
		}

		totalEnd := start + 2 + end + 2
		return totalEnd, data[start:totalEnd], nil
	})

	// Use a large buffer for frames
	const maxFrameSize = 1024 * 1024 // 1MB
	buf := make([]byte, maxFrameSize)
	scanner.Buffer(buf, maxFrameSize)

	subject := fmt.Sprintf("camera.%s.frames", p.CameraID)
	for scanner.Scan() {
		frame := scanner.Bytes()
		if p.nc != nil {
			if err := p.nc.Publish(subject, frame); err != nil {
				slog.Error("Failed to publish frame", "error", err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		slog.Error("Frame scanner error", "error", err)
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
