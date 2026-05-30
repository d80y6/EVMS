package main

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultIngestConfig()
	if config.MetricsAddr != ":2112" {
		t.Errorf("default MetricsAddr = %q, want %q", config.MetricsAddr, ":2112")
	}
}

func TestConfigValidation(t *testing.T) {
	t.Run("missing camera ID fails", func(t *testing.T) {
		config := IngestConfig{CameraID: "", RTSPURL: "rtsp://test"}
		if err := config.Validate(); err == nil {
			t.Error("expected validation error with empty CameraID")
		}
	})

	t.Run("missing RTSP URL fails", func(t *testing.T) {
		config := IngestConfig{CameraID: "cam1", RTSPURL: ""}
		if err := config.Validate(); err == nil {
			t.Error("expected validation error with empty RTSPURL")
		}
	})

	t.Run("valid config passes", func(t *testing.T) {
		config := IngestConfig{CameraID: "cam1", RTSPURL: "rtsp://test"}
		if err := config.Validate(); err != nil {
			t.Errorf("unexpected validation error: %v", err)
		}
	})
}
