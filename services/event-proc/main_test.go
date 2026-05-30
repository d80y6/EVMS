package main

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultEventProcConfig()
	if config.NATSURL != "nats://nats:4222" {
		t.Errorf("default NATSURL = %q, want %q", config.NATSURL, "nats://nats:4222")
	}
	if config.PersonConfidenceThreshold != 0.8 {
		t.Errorf("default threshold = %f, want 0.8", config.PersonConfidenceThreshold)
	}
}

func TestShouldTriggerNotification(t *testing.T) {
	p := &EventProcessor{config: &EventProcConfig{PersonConfidenceThreshold: 0.8}}

	t.Run("person with high confidence triggers", func(t *testing.T) {
		if !p.shouldTriggerNotification(Detection{Label: "person", Confidence: 0.9}) {
			t.Error("expected notification trigger for person with 0.9 confidence")
		}
	})

	t.Run("person with low confidence does not trigger", func(t *testing.T) {
		if p.shouldTriggerNotification(Detection{Label: "person", Confidence: 0.5}) {
			t.Error("expected no trigger for person with 0.5 confidence")
		}
	})

	t.Run("non-person does not trigger even with high confidence", func(t *testing.T) {
		if p.shouldTriggerNotification(Detection{Label: "car", Confidence: 0.95}) {
			t.Error("expected no trigger for car with high confidence")
		}
	})
}

func TestExtractCameraID(t *testing.T) {
	tests := []struct {
		subject  string
		expected string
	}{
		{"camera.cam1.events", "cam1"},
		{"camera.cam42.recordings.new", "cam42"},
		{"invalid", "unknown"},
		{"", "unknown"},
	}
	for _, tt := range tests {
		if got := extractCameraID(tt.subject); got != tt.expected {
			t.Errorf("extractCameraID(%q) = %q, want %q", tt.subject, got, tt.expected)
		}
	}
}
