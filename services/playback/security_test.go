package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlaybackSecurity(t *testing.T) {
	// Setup a temporary recordings directory
	tmpDir, err := os.MkdirTemp("", "recordings")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a dummy recording
	recDir := filepath.Join(tmpDir, "cam1")
	os.MkdirAll(recDir, 0755)
	os.WriteFile(filepath.Join(recDir, "test.mp4"), []byte("video data"), 0644)

	// Create a file OUTSIDE the recordings directory
	secretFile := filepath.Join(os.TempDir(), "secret.txt")
	os.WriteFile(secretFile, []byte("sensitive info"), 0644)
	defer os.Remove(secretFile)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recordingsRoot := tmpDir
		path := filepath.Clean(r.URL.Path[len("/playback/"):])
		fullPath := filepath.Join(recordingsRoot, path)
		http.ServeFile(w, r, fullPath)
	})

	t.Run("Valid access", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/playback/cam1/test.mp4", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "video data", rr.Body.String())
	})

	t.Run("Directory traversal attempt", func(t *testing.T) {
		// We want to try to access secret.txt which is at os.TempDir()/secret.txt
		// tmpDir is also in os.TempDir() usually.

		relPath, _ := filepath.Rel(tmpDir, secretFile)
		req := httptest.NewRequest("GET", "/playback/"+relPath, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		// It should NOT be StatusOK or it should not contain the secret data
		if rr.Code == http.StatusOK {
			assert.NotEqual(t, "sensitive info", rr.Body.String(), "Traversal successful!")
		}
	})
}
