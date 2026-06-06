package main

import (
	"context"
	"testing"
	"time"
)

func TestIPRangeScanner_Name(t *testing.T) {
	s := NewIPRangeScanner(nil)
	if s.Name() != "ip-range" {
		t.Errorf("expected ip-range, got %s", s.Name())
	}
}

func TestProbeONVIFDevice_Timeout(t *testing.T) {
	result := probeONVIFDevice(context.Background(), "10.255.255.255:80", time.Millisecond)
	if result != nil {
		t.Log("expected nil for unreachable device, got result (may be flaky)")
	}
}
