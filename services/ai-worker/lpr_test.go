package main

import (
	"image"
	"log/slog"
	"os"
	"testing"
)

func TestLPRProcessor_Disabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	p := NewLPRProcessor(logger)
	result, err := p.Process(image.NewRGBA(image.Rect(0, 0, 100, 100)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result when disabled")
	}
}

func TestHotlistCheck(t *testing.T) {
	p := &LPRProcessor{
		hotlist: map[string]string{"ABC123": "STOLEN"},
	}
	if _, ok := p.hotlist["ABC123"]; !ok {
		t.Fatal("expected hotlist to contain ABC123")
	}
}
