package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"
)

type ManualIPScanner struct {
	logger *slog.Logger
}

func NewManualIPScanner(logger *slog.Logger) *ManualIPScanner {
	return &ManualIPScanner{logger: logger}
}

func (s *ManualIPScanner) Name() string { return "manual" }

func (s *ManualIPScanner) Scan(ctx context.Context, subnet string, ports []int, opts ScanOptions) (<-chan ScanResult, error) {
	ch := make(chan ScanResult)

	go func() {
		defer close(ch)

		entries := parseManualEntries(subnet)
		if len(entries) == 0 {
			select {
			case <-ctx.Done():
			case ch <- ScanResult{Error: fmt.Errorf("no valid manual entries: %s", subnet)}:
			}
			return
		}

		for _, entry := range entries {
			select {
			case <-ctx.Done():
				return
			default:
			}

			timeout := 5 * time.Second
			if opts.Timeout > 0 {
				timeout = opts.Timeout
			}

			result := probeONVIFDevice(ctx, entry, timeout)
			if result != nil {
				select {
				case <-ctx.Done():
					return
				case ch <- *result:
				}
			}
		}
	}()

	return ch, nil
}

func parseManualEntries(input string) []string {
	var entries []string
	for _, part := range strings.Split(input, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, _, err := net.SplitHostPort(part); err != nil {
			entries = append(entries, net.JoinHostPort(part, "80"))
		} else {
			entries = append(entries, part)
		}
	}
	return entries
}
