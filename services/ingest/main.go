package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// StreamProcessor manages a single FFmpeg process for a camera
type StreamProcessor struct {
	CameraID string
	URL      string
	cmd      *exec.Cmd
	cancel   context.CancelFunc
}

func NewStreamProcessor(cameraID, url string) *StreamProcessor {
	return &StreamProcessor{
		CameraID: cameraID,
		URL:      url,
	}
}

func (p *StreamProcessor) Start(ctx context.Context) error {
	childCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	// This command captures RTSP and creates fragmented MP4 segments
	// and also produces a low-res MJPEG stream for AI processing
	args := []string{
		"-rtsp_transport", "tcp",
		"-i", p.URL,
		// Stream 0: Recording (Copy)
		"-map", "0:v",
		"-c:v", "copy",
		"-f", "segment",
		"-segment_time", "60",
		"-segment_format", "mp4",
		"-reset_timestamps", "1",
		fmt.Sprintf("/recordings/%s/%%Y-%%m-%%d_%%H-%%M-%%S.mp4", p.CameraID),
		// Stream 1: AI (Low-res MJPEG)
		"-map", "0:v",
		"-s", "640x360",
		"-f", "mjpeg",
		"pipe:1",
	}

	p.cmd = exec.CommandContext(childCtx, "ffmpeg", args...)

	// Create storage directory
	os.MkdirAll(fmt.Sprintf("/recordings/%s", p.CameraID), 0755)

	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		return err
	}

	// In a real implementation, we would read from stdout and send to NATS/Shared Memory for AI
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := stdout.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	log.Printf("[%s] Starting FFmpeg process", p.CameraID)
	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	return nil
}

func (p *StreamProcessor) Wait() error {
	return p.cmd.Wait()
}

func main() {
	cameraID := os.Getenv("CAMERA_ID")
	if cameraID == "" {
		cameraID = "default_cam"
	}
	rtspURL := os.Getenv("RTSP_URL")
	if rtspURL == "" {
		log.Println("RTSP_URL not set, exiting")
		return
	}

	processor := NewStreamProcessor(cameraID, rtspURL)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := processor.Start(ctx); err != nil {
		log.Fatalf("Failed to start processor: %v", err)
	}

	go func() {
		if err := processor.Wait(); err != nil {
			log.Printf("FFmpeg process exited with error: %v", err)
		}
		stop()
	}()

	<-ctx.Done()
	log.Println("Shutting down ingest service...")
	time.Sleep(1 * time.Second)
}
