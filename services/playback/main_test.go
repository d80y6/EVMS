package main

import (
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultPlaybackConfig()
	if config.Port != ":8086" {
		t.Errorf("default Port = %q, want %q", config.Port, ":8086")
	}
	if config.RecordingsRoot != "/recordings" {
		t.Errorf("default RecordingsRoot = %q, want %q", config.RecordingsRoot, "/recordings")
	}
}

func TestNewPlaybackService(t *testing.T) {
	t.Run("fails with non-existent root", func(t *testing.T) {
		config := &PlaybackConfig{
			RecordingsRoot: "/nonexistent/path/that/does/not/exist",
			Port:           ":9999",
		}
		_, err := NewPlaybackService(config, nil, nil)
		if err == nil {
			t.Error("expected error with non-existent recordings root")
		}
	})
}

func TestValidateMP4_Valid(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := t.TempDir() + "/test.mp4"

	header := make([]byte, 1024)
	copy(header[4:8], "ftyp")

	err := os.WriteFile(f, header, 0644)
	assert.NoError(t, err)

	err = validateMP4(f, logger)
	assert.NoError(t, err)
}

func TestValidateMP4_MissingFtyp(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := t.TempDir() + "/test.mp4"

	header := make([]byte, 1024)
	// Deliberately omit "ftyp" — header[4:8] stays 0x00

	err := os.WriteFile(f, header, 0644)
	assert.NoError(t, err)

	err = validateMP4(f, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ftyp")
}

func TestValidateMP4_ReadHeaderFails(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := t.TempDir() + "/test.mp4"

	header := make([]byte, 8)
	copy(header[4:8], "ftyp")

	err := os.WriteFile(f, header, 0644)
	assert.NoError(t, err)

	err = validateMP4(f, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read header")
}

func TestValidateMP4_TooSmall(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := t.TempDir() + "/test.mp4"

	header := make([]byte, 64)
	copy(header[4:8], "ftyp")

	err := os.WriteFile(f, header, 0644)
	assert.NoError(t, err)

	err = validateMP4(f, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "file too small")
}

func TestAudioContentType(t *testing.T) {
	svc := &PlaybackService{}

	tests := []struct {
		path     string
		expected string
	}{
		{"/recordings/audio.aac", "audio/aac"},
		{"/recordings/audio.wav", "audio/wav"},
		{"/recordings/audio.mp3", "audio/mpeg"},
		{"/recordings/audio.opus", "audio/opus"},
		{"/recordings/audio.ogg", "audio/ogg"},
		{"/recordings/unknown.xyz", "audio/mpeg"},
		{"/recordings/noext", "audio/mpeg"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := svc.audioContentType(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}
