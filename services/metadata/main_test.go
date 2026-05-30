package main

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultMetadataConfig()
	if config.MaxRetries != 3 {
		t.Errorf("default MaxRetries = %d, want 3", config.MaxRetries)
	}
}
