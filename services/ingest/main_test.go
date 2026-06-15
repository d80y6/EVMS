package main

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultIngestConfig()
	if config.MetricsAddr != ":2112" {
		t.Errorf("default MetricsAddr = %q, want %q", config.MetricsAddr, ":2112")
	}
}

func TestConfigValidation(t *testing.T) {
	t.Run("missing DB URL fails", func(t *testing.T) {
		config := IngestConfig{DBURL: ""}
		if err := config.Validate(); err == nil {
			t.Error("expected validation error with empty DBURL")
		}
	})

	t.Run("valid config passes", func(t *testing.T) {
		config := IngestConfig{DBURL: "postgres://localhost:5432/db"}
		if err := config.Validate(); err != nil {
			t.Errorf("unexpected validation error: %v", err)
		}
	})
}

func TestFFmpegArgs(t *testing.T) {
	recordingsPath := "/recordings/cam1"
	rtspURL := "rtsp://192.168.1.100:554/stream1"

	args := ffmpegArgs(recordingsPath, rtspURL)

	assert.Contains(t, args, "-rtsp_transport")
	assert.Contains(t, args, "tcp")
	assert.Contains(t, args, "-i")
	assert.Contains(t, args, rtspURL)
	assert.Contains(t, args, filepath.Join(recordingsPath, "%Y%m%d_%H%M%S.mp4"))

	for i, arg := range args {
		if arg == "-i" && i+1 < len(args) {
			assert.Equal(t, rtspURL, args[i+1])
		}
	}
}

func TestStreamHealth_Snapshot(t *testing.T) {
	h := &StreamHealth{}
	h.Running = true
	h.RestartCount = 3
	h.LastError = "test error"

	snap := h.snapshot()
	assert.True(t, snap.Running)
	assert.Equal(t, 3, snap.RestartCount)
	assert.Equal(t, "test error", snap.LastError)

	// Verify snapshot is a copy
	h.Running = false
	assert.True(t, snap.Running)
}

func TestNewStreamProcessor(t *testing.T) {
	config := CameraStreamConfig{
		CameraID:      "test-cam",
		RTSPURL:       "rtsp://test:554/stream",
		RecordingsDir: "/recordings",
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	sp := NewStreamProcessor(config, nil, logger)
	assert.NotNil(t, sp)
	assert.Equal(t, "test-cam", sp.config.CameraID)
	assert.NotNil(t, sp.restartCh)
}

func TestNalType(t *testing.T) {
	// 4-byte start code, NAL type 7 (SPS)
	nal := []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00}
	assert.Equal(t, byte(7), nalType(nal))

	// 3-byte start code, NAL type 5 (IDR)
	nal = []byte{0x00, 0x00, 0x01, 0x65, 0x88, 0x84}
	assert.Equal(t, byte(5), nalType(nal))

	// No start code, NAL type 1 (non-IDR slice)
	nal = []byte{0x41, 0x9a, 0x22}
	assert.Equal(t, byte(1), nalType(nal))

	// 4-byte start code, NAL type 8 (PPS)
	nal = []byte{0x00, 0x00, 0x00, 0x01, 0x68, 0xEE, 0x3C}
	assert.Equal(t, byte(8), nalType(nal))

	// 3-byte start code, NAL type 6 (SEI)
	nal = []byte{0x00, 0x00, 0x01, 0x46, 0x05, 0x04}
	assert.Equal(t, byte(6), nalType(nal))
}
