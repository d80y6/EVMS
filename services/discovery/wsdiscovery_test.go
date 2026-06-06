package main

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestBuildProbeXML(t *testing.T) {
	xml, err := buildProbeXML()
	if err != nil {
		t.Fatal(err)
	}
	if xml == "" {
		t.Fatal("expected non-empty XML")
	}
	if !contains(xml, "Probe") {
		t.Error("expected Probe element")
	}
	if !contains(xml, "NetworkVideoTransmitter") {
		t.Error("expected NetworkVideoTransmitter type")
	}
}

func TestWSDiscoveryScanner_Name(t *testing.T) {
	s := NewWSDiscoveryScanner(nil)
	if s.Name() != "ws-discovery" {
		t.Errorf("expected ws-discovery, got %s", s.Name())
	}
}

func TestWSDiscoveryScanner_ContextCancellation(t *testing.T) {
	s := NewWSDiscoveryScanner(testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ch, err := s.Scan(ctx, "", nil, ScanOptions{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case res, ok := <-ch:
		if ok {
			t.Logf("got result: %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel not closed after context cancellation")
	}
}

func contains(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
}
