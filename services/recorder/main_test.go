package main

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultRecorderConfig()
	if config.RetentionDays != 7 {
		t.Errorf("default RetentionDays = %d, want 7", config.RetentionDays)
	}
	if config.MetricsAddr != ":2112" {
		t.Errorf("default MetricsAddr = %q, want %q", config.MetricsAddr, ":2112")
	}
}

func TestConfigValidation(t *testing.T) {
	t.Run("empty DB URL fails", func(t *testing.T) {
		config := RecorderConfig{DBURL: "", NATSURL: "nats://nats:4222"}
		if err := config.Validate(); err == nil {
			t.Error("expected validation error with empty DBURL")
		}
	})

	t.Run("empty NATS URL fails", func(t *testing.T) {
		config := RecorderConfig{DBURL: "postgres://localhost", NATSURL: ""}
		if err := config.Validate(); err == nil {
			t.Error("expected validation error with empty NATSURL")
		}
	})

	t.Run("valid config passes", func(t *testing.T) {
		config := RecorderConfig{DBURL: "postgres://localhost", NATSURL: "nats://nats:4222"}
		if err := config.Validate(); err != nil {
			t.Errorf("unexpected validation error: %v", err)
		}
	})
}
