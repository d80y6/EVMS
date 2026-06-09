package main

import (
	"encoding/json"
	"testing"
)

func TestSanitizeCameraID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"camera-1", "camera-1"},
		{"Camera_123", "Camera_123"},
		{"", ""},
		{".", ""},
		{"..", ""},
		{"  trim_me  ", "trim_me"},
		{"bad/chars/here", "badcharshere"},
		{"spaces not allowed", "spacesnotallowed"},
		{"abc-def_123", "abc-def_123"},
	}

	for _, tt := range tests {
		result := sanitizeCameraID(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeCameraID(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestExportRequestJSON(t *testing.T) {
	req := ExportRequest{
		CameraID:    "cam-1",
		StartTime:   "2026-06-01T00:00:00Z",
		EndTime:     "2026-06-01T01:00:00Z",
		Watermark:   true,
		RequestedBy: "admin",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal ExportRequest: %v", err)
	}

	var decoded ExportRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ExportRequest: %v", err)
	}

	if decoded.CameraID != req.CameraID {
		t.Errorf("CameraID mismatch: %q != %q", decoded.CameraID, req.CameraID)
	}
	if decoded.Watermark != req.Watermark {
		t.Errorf("Watermark mismatch: %v != %v", decoded.Watermark, req.Watermark)
	}
}

func TestExportResultJSON(t *testing.T) {
	result := ExportResult{
		FilePath: "/exports/export_cam-1_20260601120000.mp4",
		Checksum: "abc123def456",
		Size:     1024,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal ExportResult: %v", err)
	}

	var decoded ExportResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ExportResult: %v", err)
	}

	if decoded.FilePath != result.FilePath {
		t.Errorf("FilePath mismatch: %q != %q", decoded.FilePath, result.FilePath)
	}
	if decoded.Size != result.Size {
		t.Errorf("Size mismatch: %d != %d", decoded.Size, result.Size)
	}
}
