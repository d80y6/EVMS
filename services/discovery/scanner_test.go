package main

import (
	"testing"
)

func TestScannerInterface(t *testing.T) {
	var _ Scanner = (*WSDiscoveryScanner)(nil)
	var _ Scanner = (*IPRangeScanner)(nil)
	var _ Scanner = (*MDNSScanner)(nil)
	var _ Scanner = (*ManualIPScanner)(nil)
}

func TestManualIPScanner_ParseEntries(t *testing.T) {
	entries := parseManualEntries("10.0.0.1:80, 10.0.0.2, 10.0.0.3:554")
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0] != "10.0.0.1:80" {
		t.Errorf("expected 10.0.0.1:80, got %s", entries[0])
	}
	if entries[1] != "10.0.0.2:80" {
		t.Errorf("expected 10.0.0.2:80, got %s", entries[1])
	}
	if entries[2] != "10.0.0.3:554" {
		t.Errorf("expected 10.0.0.3:554, got %s", entries[2])
	}
}

func TestManualIPScanner_EmptyInput(t *testing.T) {
	entries := parseManualEntries("")
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestManualIPScanner_Whitespace(t *testing.T) {
	entries := parseManualEntries("  10.0.0.1:80 ,  10.0.0.2  ")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0] != "10.0.0.1:80" {
		t.Errorf("expected 10.0.0.1:80, got %s", entries[0])
	}
	if entries[1] != "10.0.0.2:80" {
		t.Errorf("expected 10.0.0.2:80, got %s", entries[1])
	}
}

func TestParseCIDR(t *testing.T) {
	ipNet, err := parseCIDR("10.0.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if ipNet == nil {
		t.Fatal("expected non-nil IPNet")
	}
}

func TestParseCIDR_Invalid(t *testing.T) {
	_, err := parseCIDR("invalid")
	if err == nil {
		t.Fatal("expected error for invalid CIDR")
	}
}

func TestCollectIPs_Size(t *testing.T) {
	ipNet, _ := parseCIDR("10.0.0.0/30")
	ips := collectIPs(ipNet)
	if len(ips) != 2 {
		t.Logf("got %d IPs from /30: %v", len(ips), ips)
	}
}

func TestCollectIPs_NoSkipForIPv6(t *testing.T) {
	ipNet, _ := parseCIDR("fd00::/126")
	ips := collectIPs(ipNet)
	if len(ips) != 4 {
		t.Logf("got %d IPs from /126: %v", len(ips), ips)
	}
}
