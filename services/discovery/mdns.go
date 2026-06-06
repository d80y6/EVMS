package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/dam-vms/dam/pkg/onvif"
)

const mdnsAddr = "224.0.0.251:5353"

type MDNSScanner struct {
	logger *slog.Logger
}

func NewMDNSScanner(logger *slog.Logger) *MDNSScanner {
	return &MDNSScanner{logger: logger}
}

func (s *MDNSScanner) Name() string { return "mdns" }

func buildMDNSQuery(service string) []byte {
	var buf []byte
	buf = append(buf, 0x00, 0x00)
	buf = append(buf, 0x00, 0x00)
	buf = append(buf, 0x00, 0x01)
	buf = append(buf, 0x00, 0x00)
	buf = append(buf, 0x00, 0x00)
	buf = append(buf, 0x00, 0x00)
	for _, label := range strings.Split(service, ".") {
		buf = append(buf, byte(len(label)))
		buf = append(buf, []byte(label)...)
	}
	buf = append(buf, 0x00)
	buf = append(buf, 0x00, 0x0C)
	buf = append(buf, 0x00, 0x01)
	return buf
}

func skipDNSName(data []byte, pos int) int {
	for pos < len(data) {
		if data[pos] == 0 {
			return pos + 1
		}
		if data[pos]&0xC0 == 0xC0 {
			return pos + 2
		}
		pos += int(data[pos]) + 1
	}
	return pos
}

func parseMDNSResponse(data []byte) []string {
	var hosts []string
	if len(data) < 12 {
		return nil
	}
	pos := 12
	qdcount := int(data[4])<<8 | int(data[5])
	for i := 0; i < qdcount && pos < len(data); i++ {
		pos = skipDNSName(data, pos)
		pos += 4
	}
	ancount := int(data[6])<<8 | int(data[7])
	for i := 0; i < ancount && pos < len(data); i++ {
		pos = skipDNSName(data, pos)
		if pos+10 > len(data) {
			break
		}
		rrtype := int(data[pos])<<8 | int(data[pos+1])
		pos += 10
		rdlength := int(data[pos-2])<<8 | int(data[pos-1])
		if rrtype == 12 && pos+rdlength <= len(data) {
			var nameParts []string
			p := pos
			for p < pos+rdlength {
				if data[p] == 0 {
					break
				}
				if data[p]&0xC0 == 0xC0 {
					break
				}
				length := int(data[p])
				p++
				if p+length > pos+rdlength {
					break
				}
				nameParts = append(nameParts, string(data[p:p+length]))
				p += length
			}
			if len(nameParts) > 0 {
				hosts = append(hosts, strings.Join(nameParts, "."))
			}
		}
		pos += rdlength
	}
	return hosts
}

func (s *MDNSScanner) Scan(ctx context.Context, subnet string, ports []int, opts ScanOptions) (<-chan ScanResult, error) {
	ch := make(chan ScanResult)

	go func() {
		defer close(ch)

		conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
		if err != nil {
			select {
			case <-ctx.Done():
			case ch <- ScanResult{Error: fmt.Errorf("mDNS listen failed: %w", err)}:
			}
			return
		}
		defer conn.Close()

		dst := &net.UDPAddr{IP: net.ParseIP("224.0.0.251"), Port: 5353}

		for _, service := range []string{"_onvif._tcp.local", "_rtsp._tcp.local"} {
			query := buildMDNSQuery(service)
			if _, err := conn.WriteTo(query, dst); err != nil {
				s.logger.Error("failed to send mDNS query", "service", service, "error", err)
			}
		}

		timeout := opts.Timeout
		if timeout == 0 {
			timeout = 3 * time.Second
		}
		conn.SetReadDeadline(time.Now().Add(timeout))

		buf := make([]byte, 65535)
		seen := make(map[string]bool)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					return
				}
				return
			}

			hostnames := parseMDNSResponse(buf[:n])
			for _, host := range hostnames {
				if seen[host] {
					continue
				}
				seen[host] = true

				deviceURL := fmt.Sprintf("http://%s/onvif/device_service", host)
				deviceTimeout := 3 * time.Second
				if opts.Timeout > 0 {
					deviceTimeout = opts.Timeout
				}
				client := onvif.NewSOAPClient(deviceTimeout, nil)

				result := ScanResult{
					IP:           host,
					XAddr:        deviceURL,
					Capabilities: make(CapabilitySet),
				}

				if info, err := onvif.GetDeviceInformation(ctx, client, deviceURL); err == nil {
					result.Manufacturer = info.Manufacturer
					result.Model = info.Model
					result.Firmware = info.FirmwareVersion
					result.SerialNumber = info.SerialNumber
				} else {
					continue
				}

				if caps, err := onvif.GetCapabilities(ctx, client, deviceURL); err == nil {
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

				if hostname, err := onvif.GetHostname(ctx, client, deviceURL); err == nil {
					result.Hostname = hostname
				}

				select {
				case <-ctx.Done():
					return
				case ch <- result:
				}
			}
		}
	}()

	return ch, nil
}
