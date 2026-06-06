package main

import (
	"context"
	"time"

	"github.com/dam-vms/dam/pkg/onvif"
)

type CapabilitySet map[string]bool

type ScanResult struct {
	IP           string        `json:"ip_address"`
	Port         int           `json:"port"`
	XAddr        string        `json:"xaddr"`
	Manufacturer string        `json:"manufacturer"`
	Model        string        `json:"model"`
	Firmware     string        `json:"firmware_version"`
	SerialNumber string        `json:"serial_number"`
	Hostname     string        `json:"hostname"`
	Capabilities CapabilitySet `json:"capabilities"`
	Error        error         `json:"-"`
}

type ScanOptions struct {
	Timeout     time.Duration
	Credentials *onvif.Credentials
}

type Scanner interface {
	Name() string
	Scan(ctx context.Context, subnet string, ports []int, opts ScanOptions) (<-chan ScanResult, error)
}
