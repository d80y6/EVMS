package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestService(cacheRoot, recordingsRoot string) *ThumbnailService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &ThumbnailService{
		config: &ThumbnailConfig{
			Port:            ":8089",
			RecordingsRoot:  recordingsRoot,
			CacheRoot:       cacheRoot,
			MetricsAddr:     ":2112",
			RequestTimeout:  30 * time.Second,
			MinInterval:     10,
			DefaultInterval: 60,
		},
		logger:        logger,
		healthHandler: nil,
	}
}

func TestDefaultThumbnailConfig(t *testing.T) {
	os.Setenv("THUMBNAILS_PORT", ":9090")
	defer os.Unsetenv("THUMBNAILS_PORT")

	cfg := DefaultThumbnailConfig()
	assert.Equal(t, ":9090", cfg.Port)
	assert.Equal(t, "/recordings", cfg.RecordingsRoot)
	assert.Equal(t, "/cache/thumbnails", cfg.CacheRoot)
	assert.Equal(t, ":2112", cfg.MetricsAddr)
	assert.Equal(t, 30*time.Second, cfg.RequestTimeout)
	assert.Equal(t, 10, cfg.MinInterval)
	assert.Equal(t, 60, cfg.DefaultInterval)
}

func TestCachePath(t *testing.T) {
	svc := newTestService("/cache", "/recordings")
	ts := time.Date(2026, 6, 12, 15, 4, 5, 0, time.UTC)
	path := svc.cachePath("cam-1", ts)
	assert.Equal(t, "/cache/cam-1/20260612_150405.jpg", path)
}

func TestFindRecording_NoMatch(t *testing.T) {
	recordingsRoot := t.TempDir()
	svc := newTestService("/cache", recordingsRoot)
	ts := time.Date(2026, 6, 12, 15, 0, 0, 0, time.UTC)
	path := svc.findRecording("cam-1", ts)
	assert.Equal(t, "", path)
}

func TestFindRecording_ExactMatch(t *testing.T) {
	recordingsRoot := t.TempDir()
	camDir := filepath.Join(recordingsRoot, "cam-1")
	err := os.MkdirAll(camDir, 0755)
	require.NoError(t, err)

	recFile := filepath.Join(camDir, "20260612_150000_20260612_160000.mp4")
	err = os.WriteFile(recFile, []byte("dummy"), 0644)
	require.NoError(t, err)

	svc := newTestService("/cache", recordingsRoot)
	ts := time.Date(2026, 6, 12, 15, 30, 0, 0, time.UTC)
	path := svc.findRecording("cam-1", ts)
	assert.Equal(t, recFile, path)
}

func TestFindRecording_BestMatch(t *testing.T) {
	recordingsRoot := t.TempDir()
	camDir := filepath.Join(recordingsRoot, "cam-1")
	err := os.MkdirAll(camDir, 0755)
	require.NoError(t, err)

	rec1 := filepath.Join(camDir, "20260612_140000_20260612_150000.mp4")
	rec2 := filepath.Join(camDir, "20260612_150000_20260612_153000.mp4")
	rec3 := filepath.Join(camDir, "20260612_153000_20260612_160000.mp4")
	os.WriteFile(rec1, []byte("dummy"), 0644)
	os.WriteFile(rec2, []byte("dummy"), 0644)
	os.WriteFile(rec3, []byte("dummy"), 0644)

	svc := newTestService("/cache", recordingsRoot)
	ts := time.Date(2026, 6, 12, 15, 20, 0, 0, time.UTC)
	path := svc.findRecording("cam-1", ts)
	assert.Equal(t, rec2, path, "should pick the recording containing the timestamp with closest start")

	ts2 := time.Date(2026, 6, 12, 15, 45, 0, 0, time.UTC)
	path2 := svc.findRecording("cam-1", ts2)
	assert.Equal(t, rec3, path2, "should pick rec3 for timestamps in its range")
}

func TestFindRecording_BadFilename(t *testing.T) {
	recordingsRoot := t.TempDir()
	camDir := filepath.Join(recordingsRoot, "cam-1")
	err := os.MkdirAll(camDir, 0755)
	require.NoError(t, err)

	os.WriteFile(filepath.Join(camDir, "invalid.mp4"), []byte("dummy"), 0644)
	os.WriteFile(filepath.Join(camDir, "bad_format_123.mp4"), []byte("dummy"), 0644)

	svc := newTestService("/cache", recordingsRoot)
	ts := time.Date(2026, 6, 12, 15, 0, 0, 0, time.UTC)
	path := svc.findRecording("cam-1", ts)
	assert.Equal(t, "", path, "no valid recording should match")
}

func TestFindRecording_NoRecordingsDir(t *testing.T) {
	svc := newTestService("/cache", "/nonexistent/recordings")
	ts := time.Date(2026, 6, 12, 15, 0, 0, 0, time.UTC)
	path := svc.findRecording("cam-1", ts)
	assert.Equal(t, "", path)
}

func TestHandleTimeline_MissingParams(t *testing.T) {
	svc := newTestService(t.TempDir(), t.TempDir())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/thumbnails/timeline", nil)
	svc.handleTimeline(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleTimeline_WrongMethod(t *testing.T) {
	svc := newTestService(t.TempDir(), t.TempDir())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/thumbnails/timeline", nil)
	svc.handleTimeline(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleTimeline_InvalidTimeFormat(t *testing.T) {
	svc := newTestService(t.TempDir(), t.TempDir())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/thumbnails/timeline?camera_id=cam1&start=invalid&end=2026-06-12T16:00:00Z", nil)
	svc.handleTimeline(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleTimeline_InvalidEndTime(t *testing.T) {
	svc := newTestService(t.TempDir(), t.TempDir())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/thumbnails/timeline?camera_id=cam1&start=2026-06-12T15:00:00Z&end=invalid", nil)
	svc.handleTimeline(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleTimeline_ValidRequest(t *testing.T) {
	cacheRoot := t.TempDir()
	svc := newTestService(cacheRoot, t.TempDir())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/thumbnails/timeline?camera_id=cam1&start=2026-06-12T15:00:00Z&end=2026-06-12T15:05:00Z&interval=60", nil)
	svc.handleTimeline(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp timelineResponse
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Len(t, resp.Thumbnails, 6) // 15:00, 15:01, 15:02, 15:03, 15:04, 15:05
	for i, tn := range resp.Thumbnails {
		assert.Contains(t, tn.Timestamp, "2026-06-12T15:")
		if i == 0 {
			assert.Equal(t, "", tn.URL, "no cached thumbnail, URL should be empty")
		}
	}
}

func TestHandleTimeline_WithCachedThumbnails(t *testing.T) {
	cacheRoot := t.TempDir()
	svc := newTestService(cacheRoot, t.TempDir())

	camCacheDir := filepath.Join(cacheRoot, "cam1")
	err := os.MkdirAll(camCacheDir, 0755)
	require.NoError(t, err)

	os.WriteFile(filepath.Join(camCacheDir, "20260612_150000.jpg"), []byte("fake-jpeg"), 0644)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/thumbnails/timeline?camera_id=cam1&start=2026-06-12T15:00:00Z&end=2026-06-12T15:02:00Z&interval=60", nil)
	svc.handleTimeline(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp timelineResponse
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)

	require.Len(t, resp.Thumbnails, 3)
	assert.NotEqual(t, "", resp.Thumbnails[0].URL, "cached thumbnail should have a URL")
	assert.Contains(t, resp.Thumbnails[0].URL, "/thumbnails/image/cam1/20260612_150000.jpg")
}

func TestHandleTimeline_IntervalBelowMin(t *testing.T) {
	svc := newTestService(t.TempDir(), t.TempDir())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/thumbnails/timeline?camera_id=cam1&start=2026-06-12T15:00:00Z&end=2026-06-12T15:01:00Z&interval=5", nil)
	svc.handleTimeline(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp timelineResponse
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 2, len(resp.Thumbnails), "interval below min should use default (60s -> 2 thumbnails for 1 min range)")
}

func TestHandleImage_InvalidPathFormat(t *testing.T) {
	svc := newTestService(t.TempDir(), t.TempDir())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/thumbnails/image/invalid", nil)
	svc.handleImage(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleImage_WrongMethod(t *testing.T) {
	svc := newTestService(t.TempDir(), t.TempDir())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/thumbnails/image/cam1/20260612_150000.jpg", nil)
	svc.handleImage(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleImage_BadCameraID(t *testing.T) {
	svc := newTestService(t.TempDir(), t.TempDir())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/thumbnails/image/../20260612_150000.jpg", nil)
	svc.handleImage(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleImage_BadTimestamp(t *testing.T) {
	svc := newTestService(t.TempDir(), t.TempDir())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/thumbnails/image/cam1/badtimestamp.jpg", nil)
	svc.handleImage(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleImage_NotFound(t *testing.T) {
	cacheRoot := t.TempDir()
	svc := newTestService(cacheRoot, t.TempDir())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/thumbnails/image/cam1/20260612_150000.jpg", nil)
	svc.handleImage(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandleImage_ServesCachedImage(t *testing.T) {
	cacheRoot := t.TempDir()
	svc := newTestService(cacheRoot, t.TempDir())

	camCacheDir := filepath.Join(cacheRoot, "cam1")
	err := os.MkdirAll(camCacheDir, 0755)
	require.NoError(t, err)
	imgPath := filepath.Join(camCacheDir, "20260612_150000.jpg")
	err = os.WriteFile(imgPath, []byte("fake-jpeg-content"), 0644)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/thumbnails/image/cam1/20260612_150000.jpg", nil)
	svc.handleImage(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "image/jpeg", rr.Header().Get("Content-Type"))
	assert.Equal(t, "public, max-age=3600", rr.Header().Get("Cache-Control"))
	assert.Equal(t, "fake-jpeg-content", rr.Body.String())
}

func TestHandleImage_PathTraversalPrevention(t *testing.T) {
	cacheRoot := t.TempDir()
	svc := newTestService(cacheRoot, t.TempDir())

	outsideFile := filepath.Join(os.TempDir(), "thumb_outside_test.txt")
	err := os.WriteFile(outsideFile, []byte("secret"), 0644)
	require.NoError(t, err)
	defer os.Remove(outsideFile)

	camCacheDir := filepath.Join(cacheRoot, "cam1")
	err = os.MkdirAll(camCacheDir, 0755)
	require.NoError(t, err)
	err = os.Symlink(outsideFile, filepath.Join(camCacheDir, "20260612_150000.jpg"))
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/thumbnails/image/cam1/20260612_150000.jpg", nil)
	svc.handleImage(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestHandleImage_DirectoryAccessBlocked(t *testing.T) {
	cacheRoot := t.TempDir()
	svc := newTestService(cacheRoot, t.TempDir())

	// Create a directory at the exact cache path (trying to serve a dir as an image)
	camCacheDir := filepath.Join(cacheRoot, "cam1")
	err := os.MkdirAll(camCacheDir, 0755)
	require.NoError(t, err)
	dirPath := filepath.Join(camCacheDir, "20260612_150000.jpg")
	err = os.Mkdir(dirPath, 0755)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/thumbnails/image/cam1/20260612_150000.jpg", nil)
	svc.handleImage(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestJSONError_Thumbnails(t *testing.T) {
	rr := httptest.NewRecorder()
	jsonError(rr, "test error", http.StatusBadRequest)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	assert.Equal(t, "test error", body["error"])
}

func TestHandleTimeline_RFC3339NanoFormat(t *testing.T) {
	svc := newTestService(t.TempDir(), t.TempDir())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/thumbnails/timeline?camera_id=cam1&start=2026-06-12T15:00:00.123456Z&end=2026-06-12T15:01:00Z&interval=60", nil)
	svc.handleTimeline(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleTimeline_EmptyEndTime(t *testing.T) {
	svc := newTestService(t.TempDir(), t.TempDir())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/thumbnails/timeline?camera_id=cam1&start=2026-06-12T15:00:00Z", nil)
	svc.handleTimeline(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
