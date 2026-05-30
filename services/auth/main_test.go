package main

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultAuthConfig()
	if config.HTTPAddr != ":8081" {
		t.Errorf("default HTTPAddr = %q, want %q", config.HTTPAddr, ":8081")
	}
	if config.TokenExpiry == 0 {
		t.Error("default TokenExpiry should not be zero")
	}
}

func TestConfigValidation(t *testing.T) {
	config := AuthConfig{JWTSecret: nil}
	if err := config.Validate(); err == nil {
		t.Error("expected validation error with empty JWTSecret")
	}

	config.JWTSecret = []byte("valid-secret")
	if err := config.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}
