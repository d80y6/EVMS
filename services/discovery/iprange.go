package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/dam-vms/dam/pkg/onvif"
)

type IPRangeScanner struct {
	logger       *slog.Logger
	dialTimeout  time.Duration
	probeTimeout time.Duration
	concurrency  int
}

func NewIPRangeScanner(logger *slog.Logger) *IPRangeScanner {
	return &IPRangeScanner{
		logger:       logger,
		dialTimeout:  2 * time.Second,
		probeTimeout: 3 * time.Second,
		concurrency:  50,
	}
}

func (s *IPRangeScanner) Name() string { return "ip-range" }

func (s *IPRangeScanner) Scan(ctx context.Context, subnet string, ports []int, opts ScanOptions) (<-chan ScanResult, error) {
	ch := make(chan ScanResult)

	go func() {
		defer close(ch)

		ipNet, err := parseCIDR(subnet)
		if err != nil {
			select {
			case <-ctx.Done():
			case ch <- ScanResult{Error: fmt.Errorf("invalid subnet %s: %w", subnet, err)}:
			}
			return
		}

		ips := collectIPs(ipNet)
		sem := make(chan struct{}, s.concurrency)
		var wg sync.WaitGroup

		for _, ip := range ips {
			select {
			case <-ctx.Done():
				wg.Wait()
				return
			case sem <- struct{}{}:
			}

			wg.Add(1)
			go func(ip string) {
				defer wg.Done()
				defer func() { <-sem }()

				for _, port := range ports {
					select {
					case <-ctx.Done():
						return
					default:
					}

					addr := net.JoinHostPort(ip, fmt.Sprint(port))
					conn, err := net.DialTimeout("tcp", addr, s.dialTimeout)
					if err != nil {
						continue
					}
					conn.Close()

					timeout := s.probeTimeout
					if opts.Timeout > 0 {
						timeout = opts.Timeout
					}
					result := probeONVIFDevice(ctx, addr, timeout)
					if result != nil {
						select {
						case <-ctx.Done():
							return
						case ch <- *result:
						}
					}
					break
				}
			}(ip)
		}
		wg.Wait()
	}()

	return ch, nil
}

func parseCIDR(cidr string) (*net.IPNet, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	return ipNet, err
}

func collectIPs(ipNet *net.IPNet) []string {
	var ips []string
	ip := make(net.IP, len(ipNet.IP))
	copy(ip, ipNet.IP)
	ip = ip.Mask(ipNet.Mask)
	for ; ipNet.Contains(ip); incIP(ip) {
		ips = append(ips, ip.String())
	}
	if ip4 := ipNet.IP.To4(); ip4 != nil && len(ips) > 2 {
		ips = ips[1 : len(ips)-1]
	}
	return ips
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func probeONVIFDevice(ctx context.Context, addr string, timeout time.Duration) *ScanResult {
	deviceURL := "http://" + addr + "/onvif/device_service"
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := onvif.NewSOAPClient(timeout, nil)

	result := &ScanResult{
		IP:           addr,
		XAddr:        deviceURL,
		Capabilities: make(CapabilitySet),
	}

	info, err := onvif.GetDeviceInformation(probeCtx, client, deviceURL)
	if err != nil {
		return nil
	}
	result.Manufacturer = info.Manufacturer
	result.Model = info.Model
	result.Firmware = info.FirmwareVersion
	result.SerialNumber = info.SerialNumber

	if caps, err := onvif.GetCapabilities(probeCtx, client, deviceURL); err == nil {
		if caps.Analytics {
			result.Capabilities["analytics"] = true
		}
		if caps.Events {
			result.Capabilities["events"] = true
		}
		if caps.Imaging {
			result.Capabilities["imaging"] = true
		}
		if caps.Media {
			result.Capabilities["media"] = true
		}
		if caps.PTZ {
			result.Capabilities["ptz"] = true
		}
		if caps.Recording {
			result.Capabilities["recording"] = true
		}
	}

	if hostname, err := onvif.GetHostname(probeCtx, client, deviceURL); err == nil {
		result.Hostname = hostname
	}

	return result
}
