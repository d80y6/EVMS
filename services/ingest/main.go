package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/fsnotify/fsnotify"
	"github.com/nats-io/nats.go"
)

const (
	soapGetProfiles = `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
  <soap:Body>
    <trt:GetProfiles/>
  </soap:Body>
</soap:Envelope>`

	soapGetStreamUri = `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:trt="http://www.onvif.org/ver10/media/wsdl" xmlns:tt="http://www.onvif.org/ver10/schema">
  <soap:Body>
    <trt:GetStreamUri>
      <trt:StreamSetup>
        <tt:Stream>RTP-Unicast</tt:Stream>
        <tt:Transport>
          <tt:Protocol>RTSP</tt:Protocol>
        </tt:Transport>
      </trt:StreamSetup>
      <trt:ProfileToken>PROFILE_TOKEN</trt:ProfileToken>
    </trt:GetStreamUri>
  </soap:Body>
</soap:Envelope>`
)

// IngestConfig holds configuration for the ingest service
type IngestConfig struct {
	CameraID       string
	RTSPURL        string
	NATSURL        string
	MetricsAddr    string
	RecordingsDir  string
	ONVIFMode      bool
	ONVIFDeviceURL string
}

// DefaultIngestConfig returns a configuration with sensible defaults
func DefaultIngestConfig() IngestConfig {
	return IngestConfig{
		MetricsAddr:   ":2112",
		NATSURL:       "nats://nats:4222",
		RecordingsDir: "/recordings",
	}
}

// Validate checks if the configuration is valid
func (c *IngestConfig) Validate() error {
	if c.CameraID == "" {
		return errors.New("CAMERA_ID environment variable is required")
	}
	if c.ONVIFMode {
		if c.ONVIFDeviceURL == "" {
			return errors.New("ONVIF_DEVICE_URL environment variable is required when ONVIF_MODE is true")
		}
	} else {
		if c.RTSPURL == "" {
			return errors.New("RTSP_URL environment variable is required when ONVIF_MODE is false")
		}
	}
	return nil
}

func negotiateRTSPURL(ctx context.Context, deviceURL string) (string, error) {
	profilesResp, err := sendSOAP(ctx, deviceURL, soapGetProfiles)
	if err != nil {
		return "", fmt.Errorf("onvif get profiles: %w", err)
	}

	token, err := extractFirstProfileToken(profilesResp)
	if err != nil {
		return "", fmt.Errorf("extract profile token: %w", err)
	}

	streamReq := strings.ReplaceAll(soapGetStreamUri, "PROFILE_TOKEN", token)
	streamResp, err := sendSOAP(ctx, deviceURL, streamReq)
	if err != nil {
		return "", fmt.Errorf("onvif get stream uri: %w", err)
	}

	uri, err := extractStreamURI(streamResp)
	if err != nil {
		return "", fmt.Errorf("extract stream uri: %w", err)
	}

	return uri, nil
}

func sendSOAP(ctx context.Context, url, body string) ([]byte, error) {
	soapCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(soapCtx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("onvif request failed with status %d: %s", resp.StatusCode, string(data))
	}

	return data, nil
}

func extractFirstProfileToken(data []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "Profiles" {
			for _, attr := range start.Attr {
				if attr.Name.Local == "token" {
					return attr.Value, nil
				}
			}
		}
	}
	return "", errors.New("no profile token found in ONVIF response")
}

func extractStreamURI(data []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "Uri" {
			charToken, err := decoder.Token()
			if err != nil {
				return "", err
			}
			if char, ok := charToken.(xml.CharData); ok {
				if uri := strings.TrimSpace(string(char)); uri != "" {
					return uri, nil
				}
			}
		}
	}
	return "", errors.New("no stream URI found in ONVIF response")
}

// StreamProcessor handles video stream processing
type StreamProcessor struct {
	config IngestConfig
	cmd    *exec.Cmd
	cancel context.CancelFunc
	nc     *nats.Conn
	logger *slog.Logger
	wg     sync.WaitGroup
}

// NewStreamProcessor creates a new stream processor
func NewStreamProcessor(config IngestConfig, nc *nats.Conn, logger *slog.Logger) *StreamProcessor {
	return &StreamProcessor{
		config: config,
		nc:     nc,
		logger: logger,
	}
}

// Start begins stream processing
func (p *StreamProcessor) Start(ctx context.Context) error {
	childCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	recordingsPath := filepath.Join(p.config.RecordingsDir, p.config.CameraID)
	if err := os.MkdirAll(recordingsPath, 0755); err != nil {
		return fmt.Errorf("failed to create recordings directory: %w", err)
	}

	// Optimized FFmpeg args for low latency and consistent segmenting
	args := []string{
		"-rtsp_transport", "tcp",
		"-hwaccel", "auto",
		"-i", p.config.RTSPURL,
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

	p.wg.Add(3)
	go func() {
		defer p.wg.Done()
		p.publishMJPEG(mjpegR, fmt.Sprintf("camera.%s.frames", p.config.CameraID))
	}()
	go func() {
		defer p.wg.Done()
		p.publishH264(h264R, fmt.Sprintf("camera.%s.h264", p.config.CameraID))
	}()
	go func() {
		defer p.wg.Done()
		p.watchRecordings(childCtx, recordingsPath)
	}()

	common.StreamActive.WithLabelValues(p.config.CameraID).Set(1)

	p.logger.Info("Starting FFmpeg process",
		"camera_id", p.config.CameraID,
		"url", p.config.RTSPURL)

	if err := p.cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}
	return nil
}

// Stop gracefully stops the stream processor
func (p *StreamProcessor) Stop() error {
	if p.cancel != nil {
		p.cancel()
	}

	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}

	p.wg.Wait()

	if p.nc != nil {
		p.nc.Close()
	}

	common.StreamActive.WithLabelValues(p.config.CameraID).Set(0)
	p.logger.Info("Stream processor stopped", "camera_id", p.config.CameraID)
	return nil
}

// Wait waits for the FFmpeg process to complete
func (p *StreamProcessor) Wait() error {
	return p.cmd.Wait()
}

// watchRecordings monitors the recordings directory for new files
func (p *StreamProcessor) watchRecordings(ctx context.Context, path string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		p.logger.Error("Failed to create fsnotify watcher", "error", err)
		return
	}
	defer watcher.Close()

	if err := watcher.Add(path); err != nil {
		p.logger.Error("Failed to add watch", "path", path, "error", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Create) && filepath.Ext(event.Name) == ".mp4" {
				p.notifyNewRecording(event.Name)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			p.logger.Error("Watcher error", "error", err)
		}
	}
}

// notifyNewRecording publishes a notification about a new recording
func (p *StreamProcessor) notifyNewRecording(filePath string) {
	if p.nc == nil {
		return
	}

	eventData := map[string]string{
		"camera_id": p.config.CameraID,
		"path":      filePath,
	}
	data, err := json.Marshal(eventData)
	if err != nil {
		p.logger.Error("Failed to marshal event data", "error", err)
		return
	}

	subject := fmt.Sprintf("camera.%s.recordings.new", p.config.CameraID)
	if err := p.nc.Publish(subject, data); err != nil {
		p.logger.Error("Failed to publish recording event", "error", err)
	}
}

// publishMJPEG reads and publishes MJPEG frames
func (p *StreamProcessor) publishMJPEG(r io.Reader, subject string) {
	defer func() {
		if rc, ok := r.(io.ReadCloser); ok {
			rc.Close()
		}
	}()

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

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
				p.logger.Debug("Failed to publish MJPEG frame", "error", err)
			}
			common.FramesProcessed.WithLabelValues(p.config.CameraID, "mjpeg").Inc()
		}
	}
}

// publishH264 reads and publishes H264 NAL units
func (p *StreamProcessor) publishH264(r io.Reader, subject string) {
	defer func() {
		if rc, ok := r.(io.ReadCloser); ok {
			rc.Close()
		}
	}()

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}

		if i := bytes.Index(data, []byte{0x00, 0x00, 0x01}); i >= 0 {
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
			if err := p.nc.Publish(subject, scanner.Bytes()); err != nil {
				p.logger.Debug("Failed to publish H264 frame", "error", err)
			}
			common.FramesProcessed.WithLabelValues(p.config.CameraID, "h264").Inc()
		}
	}
}

// IngestService manages the ingest service lifecycle
type IngestService struct {
	config    IngestConfig
	logger    *slog.Logger
	processor *StreamProcessor
	nc        *nats.Conn
	healthSrv *http.Server
}

// NewIngestService creates a new ingest service
func NewIngestService(config IngestConfig, logger *slog.Logger) (*IngestService, error) {
	var nc *nats.Conn
	var err error

	if config.NATSURL != "" {
		nc, err = nats.Connect(config.NATSURL,
			nats.RetryOnFailedConnect(true),
			nats.MaxReconnects(-1),
			nats.ReconnectWait(2*time.Second),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to NATS: %w", err)
		}
		logger.Info("Connected to NATS", "url", config.NATSURL)
	}

	h := common.NewHealthHandler()
	if nc != nil {
		h.AddNATSChecker(nc, "nats")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.Liveness)
	mux.HandleFunc("/ready", h.Readiness)

	return &IngestService{
		config:    config,
		logger:    logger,
		nc:        nc,
		healthSrv: &http.Server{Addr: ":8092", Handler: mux},
	}, nil
}

// Close gracefully shuts down the service
func (s *IngestService) Close() error {
	if s.healthSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.healthSrv.Shutdown(ctx); err != nil {
			s.logger.Error("Health server shutdown error", "error", err)
		}
	}

	if s.processor != nil {
		return s.processor.Stop()
	}
	if s.nc != nil {
		s.nc.Close()
	}
	return nil
}

// Start starts the ingest service
func (s *IngestService) Start(ctx context.Context) error {
	s.processor = NewStreamProcessor(s.config, s.nc, s.logger)
	return s.processor.Start(ctx)
}

// Wait waits for the service to complete
func (s *IngestService) Wait() error {
	if s.processor != nil {
		return s.processor.Wait()
	}
	return nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultIngestConfig()
	config.CameraID = os.Getenv("CAMERA_ID")
	config.RTSPURL = os.Getenv("RTSP_URL")
	config.ONVIFMode = os.Getenv("ONVIF_MODE") == "true"
	config.ONVIFDeviceURL = os.Getenv("ONVIF_DEVICE_URL")

	if config.ONVIFMode {
		logger.Info("ONVIF mode enabled, negotiating RTSP URL",
			"device_url", config.ONVIFDeviceURL)
		negotiatedURL, err := negotiateRTSPURL(ctx, config.ONVIFDeviceURL)
		if err != nil {
			logger.Error("ONVIF negotiation failed", "error", err)
			os.Exit(1)
		}
		config.RTSPURL = negotiatedURL
		logger.Info("ONVIF negotiation succeeded", "rtsp_url", config.RTSPURL)
	}

	if addr := os.Getenv("NATS_URL"); addr != "" {
		config.NATSURL = addr
	}
	if addr := os.Getenv("METRICS_ADDR"); addr != "" {
		config.MetricsAddr = addr
	}

	if err := config.Validate(); err != nil {
		logger.Error("Invalid configuration", "error", err)
		os.Exit(1)
	}

	common.StartMetricsServer(config.MetricsAddr)

	service, err := NewIngestService(config, logger)
	if err != nil {
		logger.Error("Failed to create ingest service", "error", err)
		os.Exit(1)
	}
	defer service.Close()

	if err := service.Start(ctx); err != nil {
		logger.Error("Failed to start ingest service", "error", err)
		os.Exit(1)
	}

	go func() {
		logger.Info("Starting health HTTP server", "addr", ":8092")
		if err := service.healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Health server error", "error", err)
		}
	}()

	go func() {
		if err := service.Wait(); err != nil {
			logger.Error("FFmpeg process exited", "error", err)
		}
		stop()
	}()

	<-ctx.Done()
	logger.Info("Shutting down ingest service...")
}
