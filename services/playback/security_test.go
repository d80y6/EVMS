package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestService(t *testing.T, root string) *PlaybackService {
	t.Helper()
	config := &PlaybackConfig{
		RecordingsRoot: root,
		Port:           ":0",
	}
	s, err := NewPlaybackService(config, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPlaybackSecurity(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recordings")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(filepath.Join(tmpDir, "cam1"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "cam1", "test.mp4"), []byte("video data"), 0644)

	secretFile := filepath.Join(os.TempDir(), "secret.txt")
	os.WriteFile(secretFile, []byte("sensitive info"), 0644)
	defer os.Remove(secretFile)

	svc := newTestService(t, tmpDir)

	t.Run("Valid access", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/playback/cam1/test.mp4", nil)
		rr := httptest.NewRecorder()
		svc.handlePlaybackRequest(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "video data", rr.Body.String())
	})

	t.Run("Directory traversal via ../..", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/playback/../../"+filepath.Base(os.TempDir())+"/secret.txt", nil)
		rr := httptest.NewRecorder()
		svc.handlePlaybackRequest(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Directory traversal via encoded", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/playback/cam1/..%2f..%2f..%2fetc%2fpasswd", nil)
		rr := httptest.NewRecorder()
		svc.handlePlaybackRequest(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Directory traversal via absolute path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/playback/"+filepath.Join("..", "..", "tmp", "secret.txt"), nil)
		rr := httptest.NewRecorder()
		svc.handlePlaybackRequest(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Directory listing blocked", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/playback/", nil)
		rr := httptest.NewRecorder()
		svc.handlePlaybackRequest(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Non-existent file", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/playback/cam1/nonexistent.mp4", nil)
		rr := httptest.NewRecorder()
		svc.handlePlaybackRequest(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Directory access blocked", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/playback/cam1", nil)
		rr := httptest.NewRecorder()
		svc.handlePlaybackRequest(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})
}

func TestPlaybackSymlinkAttack(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recordings")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(filepath.Join(tmpDir, "cam1"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "cam1", "test.mp4"), []byte("video data"), 0644)

	symlinkTarget := filepath.Join(tmpDir, "cam1", "link.mp4")
	os.Symlink("/etc/passwd", symlinkTarget)
	defer os.Remove(symlinkTarget)

	svc := newTestService(t, tmpDir)

	t.Run("Symlink resolves outside root", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/playback/cam1/link.mp4", nil)
		rr := httptest.NewRecorder()
		svc.handlePlaybackRequest(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})
}

func TestPlaybackSymlinkInsideRoot(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recordings")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(filepath.Join(tmpDir, "cam1"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "cam1", "test.mp4"), []byte("video data"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "cam1", "original.mp4"), []byte("original data"), 0644)

	symlinkTarget := filepath.Join(tmpDir, "cam1", "linked.mp4")
	os.Symlink("original.mp4", symlinkTarget)
	defer os.Remove(symlinkTarget)

	svc := newTestService(t, tmpDir)

	t.Run("Symlink inside root is allowed", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/playback/cam1/linked.mp4", nil)
		rr := httptest.NewRecorder()
		svc.handlePlaybackRequest(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "original data", rr.Body.String())
	})
}

func TestPlaybackPathCleanup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recordings")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(filepath.Join(tmpDir, "cam1"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "cam1", "test.mp4"), []byte("video data"), 0644)

	svc := newTestService(t, tmpDir)

	t.Run("Double slash is normalized", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/playback//cam1//test.mp4", nil)
		rr := httptest.NewRecorder()
		svc.handlePlaybackRequest(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Dot segments are removed", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/playback/./cam1/./test.mp4", nil)
		rr := httptest.NewRecorder()
		svc.handlePlaybackRequest(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Empty path after clean is rejected", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/playback/././.", nil)
		rr := httptest.NewRecorder()
		svc.handlePlaybackRequest(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})
}
