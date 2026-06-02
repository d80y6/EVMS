package main

import (
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
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
)

// DefaultBitrate is the default bitrate in Kbps for H.264 1080p
const DefaultBitrate = 4096

// DefaultPrerecordSeconds is the default number of seconds for pre-recording buffer
const DefaultPrerecordSeconds = 5

// RecorderConfig holds configuration for the recorder service
type RecorderConfig struct {
	DBURL           string
	NATSURL         string
	MetricsAddr     string
	RetentionDays   int
	CleanupInterval time.Duration
}

// DefaultRecorderConfig returns a configuration with sensible defaults
func DefaultRecorderConfig() RecorderConfig {
	return RecorderConfig{
		MetricsAddr:     ":2112",
		NATSURL:         "nats://nats:4222",
		RetentionDays:   7,
		CleanupInterval: 1 * time.Hour,
	}
}

// Validate checks if the configuration is valid
func (c *RecorderConfig) Validate() error {
	if c.DBURL == "" {
		return errors.New("DB_URL environment variable is required")
	}
	if c.NATSURL == "" {
		return errors.New("NATS_URL environment variable is required")
	}
	return nil
}

// RecordingSegment represents a recorded video segment
type RecordingSegment struct {
	CameraID  string    `db:"camera_id"`
	StartTime time.Time `db:"start_time"`
	EndTime   time.Time `db:"end_time"`
	FilePath  string    `db:"file_path"`
	FileSize  int64     `db:"file_size"`
}

// RecordingEvent represents a NATS event for a new recording
type RecordingEvent struct {
	CameraID string `json:"camera_id"`
	Path     string `json:"path"`
}

// ringBuffer implements a pre-recording circular buffer
type ringBuffer struct {
	mu       sync.Mutex
	data     []byte
	capacity int
	head     int
	full     bool
}

func newRingBuffer(seconds int, bitrate int) *ringBuffer {
	capBytes := seconds * bitrate * 1024 / 8
	return &ringBuffer{data: make([]byte, capBytes), capacity: capBytes}
}

func (rb *ringBuffer) Write(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	for _, b := range p {
		rb.data[rb.head] = b
		rb.head = (rb.head + 1) % rb.capacity
		if rb.head == 0 {
			rb.full = true
		}
	}
	return len(p), nil
}

func (rb *ringBuffer) Bytes() []byte {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if !rb.full {
		return rb.data[:rb.head]
	}
	out := make([]byte, rb.capacity)
	copy(out, rb.data[rb.head:])
	copy(out[rb.capacity-rb.head:], rb.data[:rb.head])
	return out
}

// CameraRecorder manages per-camera recording state with pre-recording buffer
type CameraRecorder struct {
	cameraID string
	buf      *ringBuffer
}

// NewCameraRecorder creates a new camera recorder with the given prerecord duration and bitrate
func NewCameraRecorder(cameraID string, prerecordSeconds int, bitrate int) *CameraRecorder {
	return &CameraRecorder{
		cameraID: cameraID,
		buf:      newRingBuffer(prerecordSeconds, bitrate),
	}
}

// WriteNALUs processes incoming NAL units, writing to the ring buffer
func (cr *CameraRecorder) WriteNALUs(data []byte) {
	cr.buf.Write(data)
}

// FlushBuffer begins a new recording segment, flushing the buffer first
func (cr *CameraRecorder) FlushBuffer(output io.Writer) error {
	bufData := cr.buf.Bytes()
	if len(bufData) > 0 {
		if _, err := output.Write(bufData); err != nil {
			return fmt.Errorf("failed to write prerecord buffer: %w", err)
		}
	}
	return nil
}

// Recorder handles recording indexing and retention
type Recorder struct {
	db        *sqlx.DB
	logger    *slog.Logger
	config    RecorderConfig
	shard     common.ShardConfig
	sub       *nats.Subscription
	frameSub  *nats.Subscription
	mu        sync.Mutex
	cameras   map[string]*CameraRecorder
	legalHolds *LegalHoldStore
}

// NewRecorder creates a new recorder instance
func NewRecorder(ctx context.Context, config RecorderConfig, logger *slog.Logger) (*Recorder, error) {
	cb := common.NewDBCircuitBreaker("recorder")
	db, err := common.ConnectDBWithCircuitBreaker(ctx, "postgres", config.DBURL, cb)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	legalHolds := NewLegalHoldStore(db)
	if err := legalHolds.ImportLegacyHolds(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize legal holds: %w", err)
	}

	shard := common.ShardConfigFromEnv()
	if shard.IsSharded() {
		logger.Info("recorder sharding enabled",
			"shard_index", shard.ShardIndex,
			"total_shards", shard.TotalShards)
	}

	return &Recorder{
		db:         db,
		logger:     logger,
		config:     config,
		shard:      shard,
		cameras:    make(map[string]*CameraRecorder),
		legalHolds: legalHolds,
	}, nil
}

// Close gracefully shuts down the recorder
func (r *Recorder) Close() error {
	var errs []error
	if r.sub != nil {
		if err := r.sub.Unsubscribe(); err != nil {
			errs = append(errs, err)
		}
	}
	if r.frameSub != nil {
		if err := r.frameSub.Unsubscribe(); err != nil {
			errs = append(errs, err)
		}
	}
	if r.db != nil {
		if err := r.db.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors during recorder shutdown: %v", errs)
	}
	return nil
}

// IndexSegment stores a recording segment in the database
func (r *Recorder) IndexSegment(ctx context.Context, seg RecordingSegment) error {
	start := time.Now()
	query := `INSERT INTO recordings (camera_id, start_time, end_time, file_path, file_size)
              VALUES (:camera_id, :start_time, :end_time, :file_path, :file_size)`
	_, err := r.db.NamedExecContext(ctx, query, seg)
	duration := time.Since(start).Seconds()
	if err != nil {
		r.logger.Error("Failed to index segment", "error", err, "camera_id", seg.CameraID)
		return fmt.Errorf("failed to index segment: %w", err)
	}
	r.logger.Info("Indexed recording segment",
		"camera_id", seg.CameraID,
		"path", seg.FilePath,
		"size", seg.FileSize)
	common.SegmentWriteDuration.WithLabelValues(seg.CameraID).Observe(duration)
	common.RecordingsIndexed.WithLabelValues(seg.CameraID).Inc()
	return nil
}

// subscribeFrames subscribes to H264 frame data from the ingest service
func (r *Recorder) subscribeFrames(nc *nats.Conn) error {
	var err error
	r.frameSub, err = nc.Subscribe("camera.*.h264", func(msg *nats.Msg) {
		subject := msg.Subject
		parts := strings.SplitN(subject, ".", 3)
		if len(parts) < 2 {
			return
		}
		cameraID := parts[1]

		if !r.shard.OwnsCamera(cameraID) {
			return
		}

		r.mu.Lock()
		cr, ok := r.cameras[cameraID]
		if !ok {
			cr = NewCameraRecorder(cameraID, DefaultPrerecordSeconds, DefaultBitrate)
			r.cameras[cameraID] = cr
		}
		r.mu.Unlock()

		cr.WriteNALUs(msg.Data)
	})
	if err != nil {
		return err
	}
	r.frameSub.SetPendingLimits(1024, 64*1024*1024)
	return nil
}

// Listen subscribes to recording events and indexes them
func (r *Recorder) Listen(ctx context.Context, nc *nats.Conn) error {
	var err error
	r.sub, err = nc.QueueSubscribe("camera.*.recordings.new", "recorder", func(msg *nats.Msg) {
		var event RecordingEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			r.logger.Debug("Failed to unmarshal recording event", "error", err)
			return
		}

		if !r.shard.OwnsCamera(event.CameraID) {
			return
		}

		segment, err := r.processRecordingEvent(ctx, event)
		if err != nil {
			r.logger.Error("Failed to process recording event", "error", err, "path", event.Path)
			return
		}

		if err := r.IndexSegment(ctx, segment); err != nil {
			r.logger.Error("Failed to index segment", "error", err)
		}
	})
	if err != nil {
		return err
	}
	r.sub.SetPendingLimits(1024, 64*1024*1024)
	return nil
}

// flushPrerecord writes the pre-recording buffer for a camera to a companion file
func (r *Recorder) flushPrerecord(cameraID string, recordingPath string) {
	r.mu.Lock()
	cr, ok := r.cameras[cameraID]
	r.mu.Unlock()
	if !ok {
		return
	}

	prerollPath := recordingPath + ".preroll"
	f, err := os.Create(prerollPath)
	if err != nil {
		r.logger.Error("Failed to create preroll file", "path", prerollPath, "error", err)
		return
	}
	defer f.Close()

	if err := cr.FlushBuffer(f); err != nil {
		r.logger.Error("Failed to flush prerecord buffer", "camera_id", cameraID, "error", err)
	}
	r.logger.Info("Flushed prerecord buffer", "camera_id", cameraID, "path", prerollPath)
}

// processRecordingEvent processes a recording event and returns a segment
func (r *Recorder) processRecordingEvent(ctx context.Context, event RecordingEvent) (RecordingSegment, error) {
	// Wait for file to be finalized by FFmpeg
	maxRetries := 5
	var info os.FileInfo
	var err error

	for i := 0; i < maxRetries; i++ {
		info, err = os.Stat(event.Path)
		if err == nil && info.Size() > 0 {
			break
		}
		select {
		case <-ctx.Done():
			return RecordingSegment{}, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	if err != nil {
		return RecordingSegment{}, fmt.Errorf("could not stat recording file: %w", err)
	}

	filename := filepath.Base(event.Path)
	timeStr := strings.TrimSuffix(filename, ".mp4")

	// Flush prerecord buffer before returning the segment
	r.flushPrerecord(event.CameraID, event.Path)

	// Format from ingest: 20240101_120000.mp4
	startTime, err := time.Parse("20060102_150405", timeStr)
	if err != nil {
		r.logger.Debug("Could not parse timestamp from filename, using current time",
			"filename", filename, "error", err)
		startTime = time.Now()
	}

	return RecordingSegment{
		CameraID:  event.CameraID,
		StartTime: startTime,
		EndTime:   startTime.Add(60 * time.Second),
		FilePath:  event.Path,
		FileSize:  info.Size(),
	}, nil
}

// StartRetentionWorker starts a background worker for retention cleanup
func (r *Recorder) StartRetentionWorker(ctx context.Context) {
	ticker := time.NewTicker(r.config.CleanupInterval)
	defer ticker.Stop()

	r.logger.Info("Starting retention worker",
		"interval", r.config.CleanupInterval,
		"retention_days", r.config.RetentionDays)

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("Stopping retention worker")
			return
		case <-ticker.C:
			r.runRetentionCleanup(ctx)
		}
	}
}

// runRetentionCleanup removes recordings older than the retention period
func (r *Recorder) runRetentionCleanup(ctx context.Context) {
	cutoff := time.Now().AddDate(0, 0, -r.config.RetentionDays)
	r.logger.Info("Running retention cleanup", "cutoff", cutoff)

	var segments []RecordingSegment
	err := r.db.SelectContext(ctx, &segments,
		"SELECT camera_id, file_path FROM recordings WHERE start_time < $1", cutoff)
	if err != nil {
		r.logger.Error("Failed to fetch expired segments", "error", err)
		return
	}

	deletedCount := 0
	for _, seg := range segments {
		if r.legalHolds.IsOnHold(seg.CameraID) {
			r.logger.Debug("Skipping deletion for camera on legal hold", "camera_id", seg.CameraID)
			continue
		}
		if err := os.Remove(seg.FilePath); err != nil && !os.IsNotExist(err) {
			r.logger.Error("Failed to delete recording file", "path", seg.FilePath, "error", err)
			continue
		}

		if _, err := r.db.ExecContext(ctx,
			"DELETE FROM recordings WHERE file_path = $1", seg.FilePath); err != nil {
			r.logger.Error("Failed to delete recording record", "path", seg.FilePath, "error", err)
			continue
		}
		deletedCount++
	}

	r.logger.Info("Retention cleanup finished", "deleted_count", deletedCount)
}

// RecorderService manages the recorder service lifecycle
type RecorderService struct {
	config         RecorderConfig
	logger         *slog.Logger
	recorder       *Recorder
	nc             *nats.Conn
	leader         *LeaderElection
	bookmarkServer *http.Server
}

// NewRecorderService creates a new recorder service
func NewRecorderService(config RecorderConfig, logger *slog.Logger) (*RecorderService, error) {
	cb := common.NewNATSCircuitBreaker("recorder")
	nc, err := common.ConnectNATSWithCircuitBreaker(config.NATSURL, cb)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	hostname, _ := os.Hostname()
	leader, err := NewLeaderElection(nc, fmt.Sprintf("%s-%d", hostname, os.Getpid()))
	if err != nil {
		return nil, fmt.Errorf("failed to create leader election: %w", err)
	}

	return &RecorderService{
		config: config,
		logger: logger,
		nc:     nc,
		leader: leader,
	}, nil
}

// Close gracefully shuts down the service
func (s *RecorderService) Close() error {
	var errs []error
	if s.recorder != nil {
		if err := s.recorder.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.leader != nil {
		s.leader.Stop()
	}
	if s.nc != nil {
		s.nc.Close()
	}
	if s.bookmarkServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := s.bookmarkServer.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, err)
		}
		shutdownCancel()
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errs)
	}
	return nil
}

func handleDewarp(db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cameraID := r.URL.Query().Get("camera_id")
		path := r.URL.Query().Get("path")
		if cameraID == "" || path == "" {
			jsonError(w, "camera_id and path required", http.StatusBadRequest)
			return
		}

		cameraID = common.SanitizeCameraID(cameraID)
		if cameraID == "" {
			jsonError(w, "invalid camera_id", http.StatusBadRequest)
			return
		}

		if err := common.ValidateRecordingPath(path); err != nil {
			jsonError(w, "invalid path: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := common.ValidateFilePath(path, common.GetEnv("RECORDING_PATH", "/recordings")); err != nil {
			jsonError(w, "invalid path: "+err.Error(), http.StatusBadRequest)
			return
		}

		dewarpedPath := path + ".dewarped.mp4"
		if _, err := os.Stat(dewarpedPath); err == nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"path": dewarpedPath})
			return
		}

		args := []string{"-y", "-i", path, "-vf", "lenscorrection=cx=0.5:cy=0.5:k1=-0.15:k2=0.05", "-c:a", "copy", dewarpedPath}
		cmd := exec.Command("ffmpeg", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			jsonError(w, fmt.Sprintf("dewarp failed: %s", string(output)), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"path": dewarpedPath})
	}
}

// Start starts the recorder service
func (s *RecorderService) Start(ctx context.Context) error {
	recorder, err := NewRecorder(ctx, s.config, s.logger)
	if err != nil {
		return err
	}
	s.recorder = recorder

	if err := recorder.Listen(ctx, s.nc); err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}

	if err := recorder.subscribeFrames(s.nc); err != nil {
		return fmt.Errorf("failed to subscribe to frames: %w", err)
	}

	// Start background retention worker
	go recorder.StartRetentionWorker(ctx)

	// Start archive tiering manager
	tierConfig := TierConfig{
		HotPath:  common.GetEnv("RECORDING_PATH", "/recordings"),
		WarmPath: os.Getenv("WARM_STORAGE_PATH"),
		ColdPath: os.Getenv("COLD_STORAGE_PATH"),
		WarmDays: 7,
		ColdDays: 30,
	}
	tm := NewTieringManager(tierConfig, s.logger)
	go tm.Start(ctx)

	// Start leader election
	go s.leader.Start(ctx)
	s.logger.Info("Started leader election", "id", s.leader.ID())

	// Start bookmark API server
	mux := http.NewServeMux()
	healthHandler := common.NewHealthHandler()
	healthHandler.AddDBChecker(recorder.db.DB, "postgres")
	healthHandler.AddNATSChecker(s.nc, "nats")
	mux.HandleFunc("/health", healthHandler.Liveness)
	mux.HandleFunc("/ready", healthHandler.Readiness)
	mux.Handle("/storage/estimates", common.JWTAuthMiddleware(handleStorageEstimate(recorder.db)))
	mux.Handle("/bookmarks", common.JWTAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListBookmarks(recorder.db)(w, r)
		case http.MethodPost:
			handleCreateBookmark(recorder.db)(w, r)
		default:
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	mux.Handle("/legal-holds", common.JWTAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListLegalHolds(recorder.legalHolds, s.logger)(w, r)
		case http.MethodPost:
			handleCreateLegalHold(recorder.legalHolds, s.logger)(w, r)
		default:
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	mux.Handle("/legal-holds/", common.JWTAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/release") && r.Method == http.MethodPost {
			handleReleaseLegalHold(recorder.legalHolds, s.logger)(w, r)
		} else {
			jsonError(w, "not found", http.StatusNotFound)
		}
	})))
	mux.Handle("/dewarp", common.JWTAuthMiddleware(handleDewarp(recorder.db)))

	s.bookmarkServer = &http.Server{
		Addr:    ":8087",
		Handler: common.RecoveryMiddleware(mux),
	}

	go func() {
		s.logger.Info("Starting bookmark API server", "addr", ":8087")
		if err := s.bookmarkServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Bookmark server error", "error", err)
		}
	}()

	return nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := common.InitTelemetry("recorder"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultRecorderConfig()
	if dbURL := os.Getenv("DB_URL"); dbURL != "" {
		config.DBURL = dbURL
	}
	if natsURL := os.Getenv("NATS_URL"); natsURL != "" {
		config.NATSURL = natsURL
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

	service, err := NewRecorderService(config, logger)
	if err != nil {
		logger.Error("Failed to create recorder service", "error", err)
		os.Exit(1)
	}
	defer service.Close()

	if err := service.Start(ctx); err != nil {
		logger.Error("Failed to start recorder service", "error", err)
		os.Exit(1)
	}

	<-ctx.Done()
	logger.Info("Shutting down recorder service...")
}
