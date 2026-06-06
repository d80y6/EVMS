package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/google/uuid"

	"github.com/dam-vms/dam/pkg/onvif"
)

type WSDiscoveryScanner struct {
	logger *slog.Logger
}

func NewWSDiscoveryScanner(logger *slog.Logger) *WSDiscoveryScanner {
	return &WSDiscoveryScanner{logger: logger}
}

func (s *WSDiscoveryScanner) Name() string {
	return "ws-discovery"
}

type probeEnvelope struct {
	XMLName xml.Name    `xml:"http://www.w3.org/2003/05/soap-envelope Envelope"`
	Header  probeHeader `xml:"http://www.w3.org/2003/05/soap-envelope Header"`
	Body    probeBody   `xml:"http://www.w3.org/2003/05/soap-envelope Body"`
}

type probeHeader struct {
	Action    string `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing Action"`
	MessageID string `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing MessageID"`
	To        string `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing To"`
}

type probeBody struct {
	Probe probe `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery Probe"`
}

type probe struct {
	Types string `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery Types"`
}

type probeMatchEnvelope struct {
	XMLName xml.Name       `xml:"http://www.w3.org/2003/05/soap-envelope Envelope"`
	Body    probeMatchBody `xml:"http://www.w3.org/2003/05/soap-envelope Body"`
}

type probeMatchBody struct {
	ProbeMatches probeMatches `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery ProbeMatches"`
}

type probeMatches struct {
	ProbeMatch []probeMatchItem `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery ProbeMatch"`
}

type probeMatchItem struct {
	XAddrs string `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery XAddrs"`
	Types  string `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery Types"`
	Scopes string `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery Scopes"`
}

func buildProbeXML() (string, error) {
	uid := uuid.New().String()
	msgID := "uuid:" + uid
	env := probeEnvelope{
		Header: probeHeader{
			Action:    "http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe",
			MessageID: msgID,
			To:        "urn:schemas-xmlsoap-org:ws:2005:04:discovery",
		},
		Body: probeBody{
			Probe: probe{
				Types: "dn:NetworkVideoTransmitter",
			},
		},
	}
	out, err := xml.MarshalIndent(env, "", "  ")
	if err != nil {
		return "", err
	}
	return xml.Header + string(out), nil
}

func (s *WSDiscoveryScanner) Scan(ctx context.Context, subnet string, ports []int, opts ScanOptions) (<-chan ScanResult, error) {
	ch := make(chan ScanResult)

	go func() {
		defer close(ch)

		probeMsg, err := buildProbeXML()
		if err != nil {
			select {
			case <-ctx.Done():
			case ch <- ScanResult{Error: fmt.Errorf("failed to build probe: %w", err)}:
			}
			return
		}

		conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
		if err != nil {
			select {
			case <-ctx.Done():
			case ch <- ScanResult{Error: fmt.Errorf("failed to create UDP socket: %w", err)}:
			}
			return
		}
		defer conn.Close()

		multicastAddr := &net.UDPAddr{IP: net.ParseIP("239.255.255.250"), Port: 3702}
		if _, err := conn.WriteTo([]byte(probeMsg), multicastAddr); err != nil {
			s.logger.Error("failed to send WS-Discovery probe", "error", err)
			return
		}

		timeout := opts.Timeout
		if timeout == 0 {
			timeout = 5 * time.Second
		}
		conn.SetReadDeadline(time.Now().Add(timeout))

		seen := make(map[string]bool)
		buf := make([]byte, 65535)

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

			var env probeMatchEnvelope
			if err := xml.Unmarshal(buf[:n], &env); err != nil {
				continue
			}

			for _, match := range env.Body.ProbeMatches.ProbeMatch {
				if match.XAddrs == "" || seen[match.XAddrs] {
					continue
				}
				seen[match.XAddrs] = true

				addrList := bytes.Fields([]byte(match.XAddrs))
				if len(addrList) == 0 {
					continue
				}
				deviceURL := string(addrList[0])

				result := ScanResult{
					XAddr:        match.XAddrs,
					IP:           deviceURL,
					Capabilities: make(CapabilitySet),
				}

				client := onvif.NewSOAPClient(5*time.Second, nil)

				if info, err := onvif.GetDeviceInformation(ctx, client, deviceURL); err == nil {
					result.Manufacturer = info.Manufacturer
					result.Model = info.Model
					result.Firmware = info.FirmwareVersion
					result.SerialNumber = info.SerialNumber
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
