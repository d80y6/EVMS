package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/dam-vms/dam/pkg/onvif"
	"github.com/fsnotify/fsnotify"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
)

const (
	discoveryInterval = 30 * time.Second
)

// IngestConfig holds shared configuration for the ingest service
type IngestConfig struct {
	DBURL         string
	NATSURL       string
	MetricsAddr   string
	RecordingsDir string
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
	if c.DBURL == "" {
		return errors.New("DB_URL environment variable is required")
	}
	return nil
}

// DBCamera represents a camera row from the database
type DBCamera struct {
	ID             string  `db:"id"`
	Name           string  `db:"name"`
	ConnectionURL  string  `db:"connection_url"`
	Status         string  `db:"status"`
	OnvifUsername  *string `db:"onvif_username"`
	OnvifPassword  *string `db:"onvif_password"`
}

func negotiateRTSPURL(ctx context.Context, deviceURL string, username, password string) (string, error) {
	if username == "" || password == "" {
		return deviceURL, nil
	}

	creds := &onvif.Credentials{Username: username, Password: password}
	client := onvif.NewSOAPClient(15*time.Second, creds)
	mediaURL := onvif.BuildMediaURL(deviceURL)

	profiles, err := onvif.GetProfiles(ctx, client, mediaURL)
	if err != nil {
		return deviceURL, fmt.Errorf("onvif get profiles (falling back to direct URL): %w", err)
	}

	mainProfile := onvif.FindMainProfile(profiles)
	if mainProfile == nil {
		if len(profiles) == 0 {
			return deviceURL, errors.New("no ONVIF profiles found (falling back to direct URL)")
		}
		mainProfile = &profiles[0]
	}

	uri, err := onvif.GetStreamURIForProfileToken(ctx, client, deviceURL, mainProfile.Token)
	if err != nil {
		return deviceURL, fmt.Errorf("onvif get stream uri (falling back to direct URL): %w", err)
	}

	return uri, nil
}

// CameraStreamConfig holds per-camera stream configuration
type CameraStreamConfig struct {
	CameraID      string
	RTSPURL       string
	RecordingsDir string
}

// StreamProcessor handles video stream processing for a single camera
type StreamProcessor struct {
	config     CameraStreamConfig
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	nc         *nats.Conn
	logger     *slog.Logger
	wg         sync.WaitGroup
	mu         sync.Mutex
	restartCh  chan struct{}
	running    bool
	health     StreamHealth
}

type StreamHealth struct {
	mu           sync.RWMutex
	Running      bool      `json:"running"`
	Uptime       time.Time `json:"uptime"`
	RestartCount int       `json:"restart_count"`
	LastError    string    `json:"last_error,omitempty"`
	LastRestart  time.Time `json:"last_restart,omitempty"`
}

func (h *StreamHealth) snapshot() StreamHealth {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return StreamHealth{
		Running:      h.Running,
		Uptime:       h.Uptime,
		RestartCount: h.RestartCount,
		LastError:    h.LastError,
		LastRestart:  h.LastRestart,
	}
}

// NewStreamProcessor creates a new stream processor
func NewStreamProcessor(config CameraStreamConfig, nc *nats.Conn, logger *slog.Logger) *StreamProcessor {
	return &StreamProcessor{
		config:    config,
		nc:        nc,
		logger:    logger,
		restartCh: make(chan struct{}, 1),
		health:    StreamHealth{},
	}
}

func ffmpegArgs(recordingsPath, rtspURL string) []string {
	return []string{
		"-rtsp_transport", "tcp",
		"-fflags", "nobuffer",
		"-flags", "low_delay",
		"-hwaccel", "auto",
		"-i", rtspURL,
		"-map", "0:v:0",
		"-c:v:0", "copy",
		"-f", "segment",
		"-segment_time", "60",
		"-segment_format", "mp4",
		"-segment_atclocktime", "1",
		"-reset_timestamps", "1",
		"-strftime", "1",
		filepath.Join(recordingsPath, "%Y%m%d_%H%M%S.mp4"),
		"-map", "0:v:0",
		"-s", "640x360",
		"-f", "mjpeg",
		"-q:v", "5",
		"-fpsmax", "10",
		"pipe:3",
		"-map", "0:v:0",
		"-c:v", "copy",
		"-f", "h264",
		"-bsf:v", "h264_mp4toannexb",
		"pipe:4",
	}
}

// runFFmpeg launches ffmpeg and the associated pipeline goroutines.
// It returns when ffmpeg exits.
func (p *StreamProcessor) runFFmpeg(ctx context.Context) error {
	recordingsPath := filepath.Join(p.config.RecordingsDir, p.config.CameraID)
	if err := os.MkdirAll(recordingsPath, 0755); err != nil {
		return fmt.Errorf("failed to create recordings directory: %w", err)
	}

	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(childCtx, "ffmpeg", ffmpegArgs(recordingsPath, p.config.RTSPURL)...)

	mjpegR, mjpegW, _ := os.Pipe()
	h264R, h264W, _ := os.Pipe()
	cmd.ExtraFiles = []*os.File{mjpegW, h264W}

	ffWg := sync.WaitGroup{}
	ffWg.Add(3)
	go func() {
		defer ffWg.Done()
		p.publishMJPEG(mjpegR, fmt.Sprintf("camera.%s.frames", p.config.CameraID))
		mjpegR.Close()
	}()
	go func() {
		defer ffWg.Done()
		p.publishH264(h264R, fmt.Sprintf("camera.%s.h264", p.config.CameraID))
		h264R.Close()
	}()
	go func() {
		defer ffWg.Done()
		p.watchRecordings(childCtx, recordingsPath)
	}()

	p.logger.Info("Starting FFmpeg process",
		"camera_id", p.config.CameraID,
		"url", p.config.RTSPURL)

	if err := cmd.Start(); err != nil {
		mjpegW.Close()
		h264W.Close()
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	p.mu.Lock()
	p.cmd = cmd
	p.mu.Unlock()

	mjpegW.Close()
	h264W.Close()

	err := cmd.Wait()
	ffWg.Wait()
	return err
}

// StartSupervisor launches the supervised ffmpeg lifecycle with exponential backoff restarts.
func (p *StreamProcessor) StartSupervisor(ctx context.Context) {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	p.mu.Unlock()

	backoff := 1 * time.Second
	maxBackoff := 5 * time.Minute
	cameraID := p.config.CameraID

	p.health.mu.Lock()
	p.health.Running = true
	p.health.Uptime = time.Now()
	p.mu.Unlock()

	for {
		start := time.Now()
		err := p.runFFmpeg(ctx)
		duration := time.Since(start)

		p.health.mu.Lock()
		p.health.RestartCount++
		p.health.LastRestart = time.Now()
		if err != nil {
			p.health.LastError = err.Error()
		} else {
			p.health.LastError = "ffmpeg exited"
		}
		p.health.mu.Unlock()

		select {
		case <-ctx.Done():
			p.logger.Info("Stream processor stopped", "camera_id", cameraID,
				"uptime", duration.Round(time.Second), "restarts", p.health.RestartCount)
			common.StreamActive.WithLabelValues(cameraID).Set(0)
			p.health.mu.Lock()
			p.health.Running = false
			p.health.mu.Unlock()
			return
		default:
		}

		if err != nil {
			p.logger.Warn("FFmpeg exited, restarting",
				"camera_id", cameraID, "uptime", duration.Round(time.Second),
				"error", err, "backoff", backoff.Round(time.Second))
		} else {
			p.logger.Warn("FFmpeg exited unexpectedly, restarting",
				"camera_id", cameraID, "uptime", duration.Round(time.Second),
				"backoff", backoff.Round(time.Second))
		}

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			common.StreamActive.WithLabelValues(cameraID).Set(0)
			return
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// Stop gracefully stops the stream processor
func (p *StreamProcessor) Stop() error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return nil
	}
	p.running = false
	if p.cancel != nil {
		p.cancel()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
	p.mu.Unlock()

	common.StreamActive.WithLabelValues(p.config.CameraID).Set(0)
	p.logger.Info("Stream processor stopped", "camera_id", p.config.CameraID)
	return nil
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

	var (
		lastFrameTime = time.Now()
		bytesInWindow int64
		windowStart   = time.Now()
	)

	for scanner.Scan() {
		if p.nc != nil {
			frame := scanner.Bytes()
			now := time.Now()

			common.StreamLatencyMs.WithLabelValues(p.config.CameraID).Set(float64(now.Sub(lastFrameTime).Milliseconds()))
			lastFrameTime = now

			bytesInWindow += int64(len(frame))
			if now.Sub(windowStart) >= time.Second {
				bps := float64(bytesInWindow) * 8 / now.Sub(windowStart).Seconds()
				common.StreamBitrate.WithLabelValues(p.config.CameraID).Set(bps)
				bytesInWindow = 0
				windowStart = now
			}

			if err := p.nc.Publish(subject, frame); err != nil {
				p.logger.Debug("Failed to publish MJPEG frame", "error", err)
				common.FramesDroppedTotal.WithLabelValues(p.config.CameraID, "publish_error").Inc()
			} else {
				common.FramesProcessed.WithLabelValues(p.config.CameraID, "mjpeg").Inc()
			}
		}
	}
	if err := scanner.Err(); err != nil {
		p.logger.Error("MJPEG scanner error", "error", err)
	}
}

// nalType returns the H264 NAL unit type, skipping the Annex B start code prefix.
func nalType(nal []byte) byte {
	offset := 0
	if len(nal) >= 4 && nal[0] == 0 && nal[1] == 0 && nal[2] == 0 && nal[3] == 1 {
		offset = 4
	} else if len(nal) >= 3 && nal[0] == 0 && nal[1] == 0 && nal[2] == 1 {
		offset = 3
	} else {
		return nal[0] & 0x1F
	}
	return nal[offset] & 0x1F
}

// nalStartCodeIndex finds the next Annex B start code prefix (3 or 4 byte).
// Returns -1 if not found. Checks 4-byte first so it doesn't match the
// 3-byte suffix inside a 4-byte start code.
func nalStartCodeIndex(data []byte) int {
	if i := bytes.Index(data, []byte{0x00, 0x00, 0x00, 0x01}); i >= 0 {
		return i
	}
	if i := bytes.Index(data, []byte{0x00, 0x00, 0x01}); i >= 0 {
		return i
	}
	return -1
}

// nalStartCodeLen returns the length of the Annex B start code at the beginning of data.
func nalStartCodeLen(data []byte) int {
	if len(data) >= 4 && data[0] == 0 && data[1] == 0 && data[2] == 0 && data[3] == 1 {
		return 4
	}
	if len(data) >= 3 && data[0] == 0 && data[1] == 0 && data[2] == 1 {
		return 3
	}
	return 0
}

// readUE decodes an unsigned Exponential-Golomb code from the given bit offset.
func readUE(nal []byte, bitOffset int) (int, int) {
	leadingZeros := 0
	for bitOffset/8 < len(nal) {
		if nal[bitOffset/8]>>(7-bitOffset%8)&1 == 0 {
			leadingZeros++
			bitOffset++
		} else {
			bitOffset++
			break
		}
	}
	value := 0
	for i := 0; i < leadingZeros; i++ {
		if bitOffset/8 >= len(nal) {
			break
		}
		bitOffset++
		byteIdx := bitOffset / 8
		bitIdx := bitOffset % 8
		value = (value << 1) | int(nal[byteIdx]>>(7-bitIdx)&1)
	}
	return (1 << leadingZeros) - 1 + value, bitOffset
}

// isFirstSliceOfFrame returns true if the NAL unit is the first slice of a new
// H264 access unit (frame). It parses the slice header's first_mb_in_slice field.
func isFirstSliceOfFrame(nal []byte) bool {
	offset := 0
	if len(nal) >= 4 && nal[0] == 0 && nal[1] == 0 && nal[2] == 0 && nal[3] == 1 {
		offset = 4
	} else if len(nal) >= 3 && nal[0] == 0 && nal[1] == 0 && nal[2] == 1 {
		offset = 3
	} else {
		return false
	}
	t := nal[offset] & 0x1F
	if t != 1 && t != 5 {
		return false
	}
	// Slice header starts at bit offset (offset+1)*8 (after the NAL header byte)
	fms, _ := readUE(nal, (offset+1)*8)
	return fms == 0
}

// publishH264 reads and publishes H264 frames (batched NAL units)
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

		// search for any Annex B start code (3 or 4 byte)
		start := nalStartCodeIndex(data)
		if start < 0 {
			if atEOF {
				return len(data), data, nil
			}
			return 0, nil, nil
		}

		// search for the next start code after this one
		rest := data[start+nalStartCodeLen(data[start:]):]
		next := nalStartCodeIndex(rest)
		if next < 0 {
			if atEOF {
				return len(data), data, nil
			}
			return 0, nil, nil
		}

		end := start + nalStartCodeLen(data[start:]) + next
		return end, data[start:end], nil
	})

	var (
		lastFrameTime = time.Now()
		bytesInWindow int64
		windowStart   = time.Now()
		frameBuf      []byte
		hasSlice      bool
	)

	for scanner.Scan() {
		if p.nc == nil {
			continue
		}
		nal := scanner.Bytes()
		now := time.Now()

		common.StreamLatencyMs.WithLabelValues(p.config.CameraID).Set(float64(now.Sub(lastFrameTime).Milliseconds()))
		lastFrameTime = now

		bytesInWindow += int64(len(nal))
		if now.Sub(windowStart) >= time.Second {
			bps := float64(bytesInWindow) * 8 / now.Sub(windowStart).Seconds()
			common.StreamBitrate.WithLabelValues(p.config.CameraID).Set(bps)
			bytesInWindow = 0
			windowStart = now
		}

		t := nalType(nal)
		isNewFrame := t == 7 || (t == 6 && hasSlice) || ((t == 1 || t == 5) && isFirstSliceOfFrame(nal))

		if isNewFrame && hasSlice {
			if err := p.nc.Publish(subject, frameBuf); err != nil {
				p.logger.Debug("Failed to publish H264 frame", "error", err)
				common.FramesDroppedTotal.WithLabelValues(p.config.CameraID, "publish_error").Inc()
			} else {
				common.FramesProcessed.WithLabelValues(p.config.CameraID, "h264").Inc()
			}
			frameBuf = nil
			hasSlice = false
		}

		frameBuf = append(frameBuf, nal...)
		if t == 1 || t == 5 {
			hasSlice = true
		}
	}

	// Publish any remaining buffered data
	if len(frameBuf) > 0 && p.nc != nil {
		if err := p.nc.Publish(subject, frameBuf); err != nil {
			p.logger.Debug("Failed to publish remaining H264 frame", "error", err)
		} else {
			common.FramesProcessed.WithLabelValues(p.config.CameraID, "h264").Inc()
		}
	}

	if err := scanner.Err(); err != nil {
		p.logger.Error("H264 scanner error", "camera_id", p.config.CameraID, "error", err)
	} else {
		p.logger.Info("H264 scanner finished", "camera_id", p.config.CameraID)
	}
}

// IngestService manages the ingest service lifecycle for multiple cameras
type IngestService struct {
	config     IngestConfig
	db         *sqlx.DB
	logger     *slog.Logger
	processors map[string]*StreamProcessor
	nc         *nats.Conn
	healthSrv  *http.Server
	mu         sync.Mutex
	discCancel context.CancelFunc
}

// NewIngestService creates a new ingest service
func NewIngestService(ctx context.Context, config IngestConfig, logger *slog.Logger) (*IngestService, error) {
	var nc *nats.Conn
	var err error

	if config.NATSURL != "" {
		nc, err = nats.Connect(config.NATSURL, append(common.NATSTLSOptions(),
			nats.RetryOnFailedConnect(true),
			nats.MaxReconnects(-1),
			nats.ReconnectWait(2*time.Second),
		)...)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to NATS: %w", err)
		}
		logger.Info("Connected to NATS", "url", config.NATSURL)
	}

	cb := common.NewDBCircuitBreaker("ingest")
	db, err := common.ConnectDBWithCircuitBreaker(ctx, "postgres", config.DBURL, cb)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	logger.Info("Connected to database")

	h := common.NewHealthHandler()
	if nc != nil {
		h.AddNATSChecker(nc, "nats")
	}
	if db != nil {
		h.AddDBChecker(db.DB, "postgres")
	}

	svc := &IngestService{
		config:     config,
		db:         db,
		logger:     logger,
		nc:         nc,
		processors: make(map[string]*StreamProcessor),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.Liveness)
	mux.HandleFunc("/ready", h.Readiness)
	mux.HandleFunc("/health/camera/", svc.handleCameraHealth)
	svc.healthSrv = &http.Server{Addr: ":8092", Handler: mux}

	return svc, nil
}

func (s *IngestService) handleCameraHealth(w http.ResponseWriter, r *http.Request) {
	cameraID := strings.TrimPrefix(r.URL.Path, "/health/camera/")
	if cameraID == "" {
		http.Error(w, "camera_id required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	proc, exists := s.processors[cameraID]
	s.mu.Unlock()

	if !exists {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"camera_id": cameraID,
			"running":   false,
			"error":     "no active processor",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(proc.health.snapshot())
}

// discoverCameras queries the database and starts/stops processors accordingly
func (s *IngestService) discoverCameras(ctx context.Context) {
	var cameras []DBCamera
	if err := s.db.SelectContext(ctx, &cameras,
		"SELECT id, name, connection_url, status, onvif_username, onvif_password FROM cameras WHERE connection_url IS NOT NULL AND connection_URL != ''",
	); err != nil {
		s.logger.Error("Failed to query cameras", "error", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	active := make(map[string]bool)
	for _, cam := range cameras {
		if cam.ConnectionURL == "" {
			continue
		}
		active[cam.ID] = true

		if _, exists := s.processors[cam.ID]; exists {
			continue
		}

		username := ""
		password := ""
		if cam.OnvifUsername != nil {
			username = *cam.OnvifUsername
		}
		if cam.OnvifPassword != nil {
			password = *cam.OnvifPassword
		}

		rtspURL, err := negotiateRTSPURL(ctx, cam.ConnectionURL, username, password)
		if err != nil {
			s.logger.Warn("ONVIF negotiation failed, using direct RTSP URL",
				"camera_id", cam.ID,
				"error", err)
		}

		proc := NewStreamProcessor(CameraStreamConfig{
			CameraID:      cam.ID,
			RTSPURL:       rtspURL,
			RecordingsDir: s.config.RecordingsDir,
		}, s.nc, s.logger)

		s.processors[cam.ID] = proc
		go proc.StartSupervisor(ctx)
		s.logger.Info("Started ingesting camera",
			"camera_id", cam.ID,
			"name", cam.Name,
			"url", cam.ConnectionURL)
	}

	for id, proc := range s.processors {
		if !active[id] {
			proc.Stop()
			delete(s.processors, id)
			s.logger.Info("Stopped ingesting camera (removed from DB)", "camera_id", id)
		}
	}
}

// discoveryLoop periodically discovers cameras from the DB
func (s *IngestService) discoveryLoop(ctx context.Context) {
	s.discoverCameras(ctx)
	ticker := time.NewTicker(discoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.discoverCameras(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// Close gracefully shuts down the service
func (s *IngestService) Close() error {
	if s.discCancel != nil {
		s.discCancel()
	}

	if s.healthSrv != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.healthSrv.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("Health server shutdown error", "error", err)
		}
	}

	s.mu.Lock()
	for id, proc := range s.processors {
		proc.Stop()
		delete(s.processors, id)
	}
	s.mu.Unlock()

	if s.nc != nil {
		s.nc.Close()
	}

	if s.db != nil {
		s.db.Close()
	}
	return nil
}

// Start starts the ingest service
func (s *IngestService) Start(ctx context.Context) error {
	discCtx, discCancel := context.WithCancel(ctx)
	s.discCancel = discCancel
	go s.discoveryLoop(discCtx)
	return nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	common.CheckJWTSecret()

	if err := common.InitTelemetry("ingest"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultIngestConfig()
	config.DBURL = os.Getenv("DB_URL")

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
	common.StartResourceMonitor(ctx)

	service, err := NewIngestService(ctx, config, logger)
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

	<-ctx.Done()
	logger.Info("Shutting down ingest service...")
}
