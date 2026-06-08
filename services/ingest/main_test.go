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
