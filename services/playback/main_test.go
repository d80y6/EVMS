package main

import (
	"testing"
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
