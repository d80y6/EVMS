package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/dam-vms/dam/pkg/onvif"
)

type DiscoveryConfig struct {
	Port            string
	MetricsPort     string
	NATSURL         string
	DBURL           string
	ScanTimeout     time.Duration
	GracefulTimeout time.Duration
}

func DefaultDiscoveryConfig() *DiscoveryConfig {
	return &DiscoveryConfig{
		Port:            common.GetEnv("DISCOVERY_PORT", ":8091"),
		MetricsPort:     common.GetEnv("METRICS_ADDR", ":2112"),
		NATSURL:         os.Getenv("DISCOVERY_NATS_URL"),
		DBURL:           os.Getenv("DB_URL"),
		ScanTimeout:     5 * time.Second,
		GracefulTimeout: 30 * time.Second,
	}
}

type discoveredCamera struct {
	Address      string   `json:"url"`
	Manufacturer string   `json:"manufacturer"`
	Model        string   `json:"model"`
	Firmware     string   `json:"firmware_version"`
	SerialNumber string   `json:"serial_number"`
	Hostname     string   `json:"hostname"`
	Capabilities []string `json:"capabilities"`
	XAddrs       string   `json:"xaddrs"`
	Scopes       string   `json:"scopes"`
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

type scanStatus struct {
	Scanning bool   `json:"scanning"`
	Count    int    `json:"count"`
	Error    string `json:"error,omitempty"`
}

type DiscoveryService struct {
	config        *DiscoveryConfig
	logger        *slog.Logger
	db            *sqlx.DB
	mu            sync.RWMutex
	results       []discoveredCamera
	scanning      bool
	scanError     string
	natsConn      *nats.Conn
	server        *http.Server
	healthHandler *common.HealthHandler
}

func NewDiscoveryService(config *DiscoveryConfig, logger *slog.Logger) (*DiscoveryService, error) {
	s := &DiscoveryService{
		config:        config,
		logger:        logger,
		results:       nil,
		healthHandler: common.NewHealthHandler(),
	}

	if config.NATSURL != "" {
		nc, err := nats.Connect(config.NATSURL)
		if err != nil {
			logger.Warn("Failed to connect to NATS, proceeding without it", "error", err)
		} else {
			s.natsConn = nc
			logger.Info("Connected to NATS", "url", config.NATSURL)
		}
	}

	s.healthHandler.AddNATSChecker(s.natsConn, "nats")

	if config.DBURL != "" {
		cb := common.NewDBCircuitBreaker("discovery")
		db, err := common.ConnectDBWithCircuitBreaker(context.Background(), "postgres", config.DBURL, cb)
		if err != nil {
			logger.Warn("Failed to connect to database, proceeding without it", "error", err)
		} else {
			s.db = db
			s.healthHandler.AddDBChecker(db.DB, "postgres")
			logger.Info("Connected to database")
		}
	}

	return s, nil
}

func (s *DiscoveryService) Start() error {
	mux := http.NewServeMux()
	mux.Handle("/discovery/scan", common.JWTAuthMiddleware(s.handleScan))
	mux.Handle("/discovery/results", common.JWTAuthMiddleware(s.handleResults))
	mux.Handle("/discovery/status", common.JWTAuthMiddleware(s.handleStatus))
	mux.HandleFunc("/health", s.healthHandler.Liveness)
	mux.HandleFunc("/ready", s.healthHandler.Readiness)

	s.server = &http.Server{
		Addr:    s.config.Port,
		Handler: common.RecoveryMiddleware(mux),
	}

	go func() {
		s.logger.Info("Discovery Service started", "address", s.config.Port, "metrics_address", s.config.MetricsPort)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Discovery server error", "error", err)
		}
	}()

	return nil
}

func (s *DiscoveryService) Shutdown(ctx context.Context) error {
	if s.server != nil {
		if err := s.server.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown server: %w", err)
		}
	}
	if s.natsConn != nil {
		s.natsConn.Close()
	}
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			return fmt.Errorf("failed to close database: %w", err)
		}
	}
	return nil
}

func probeXML() (string, error) {
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

func (s *DiscoveryService) sendProbe(ctx context.Context) ([]discoveredCamera, error) {
	probeMsg, err := probeXML()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal probe: %w", err)
	}

	s.logger.Debug("Sending WS-Discovery probe")

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("failed to create UDP socket: %w", err)
	}
	defer conn.Close()

	multicastAddr := &net.UDPAddr{IP: net.ParseIP("239.255.255.250"), Port: 3702}
	if _, err := conn.WriteTo([]byte(probeMsg), multicastAddr); err != nil {
		return nil, fmt.Errorf("failed to send probe: %w", err)
	}

	s.logger.Info("Sent WS-Discovery probe, waiting for responses...")

	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		deadline = time.Now().Add(s.config.ScanTimeout)
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, fmt.Errorf("failed to set read deadline: %w", err)
	}

	seen := make(map[string]bool)
	var cameras []discoveredCamera
	buf := make([]byte, 65535)

	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				break
			}
			return cameras, fmt.Errorf("error reading UDP: %w", err)
		}

		var env probeMatchEnvelope
		if err := xml.Unmarshal(buf[:n], &env); err != nil {
			s.logger.Debug("Failed to parse probe response XML", "error", err)
			continue
		}

		for _, match := range env.Body.ProbeMatches.ProbeMatch {
			if match.XAddrs == "" {
				continue
			}
			if seen[match.XAddrs] {
				continue
			}
			seen[match.XAddrs] = true

			addrList := bytes.Fields([]byte(match.XAddrs))
			if len(addrList) == 0 {
				continue
			}
			deviceURL := string(addrList[0])

			client := onvif.NewSOAPClient(5*time.Second, nil)
			cam := discoveredCamera{
				Address: deviceURL,
				XAddrs:  match.XAddrs,
				Scopes:  match.Scopes,
			}

			if info, err := onvif.GetDeviceInformation(ctx, client, deviceURL); err == nil {
				cam.Manufacturer = info.Manufacturer
				cam.Model = info.Model
				cam.Firmware = info.FirmwareVersion
				cam.SerialNumber = info.SerialNumber
			} else {
				s.logger.Debug("GetDeviceInformation failed", "device", deviceURL, "error", err)
			}

			if caps, err := onvif.GetCapabilities(ctx, client, deviceURL); err == nil {
				if caps.Analytics {
					cam.Capabilities = append(cam.Capabilities, "analytics")
				}
				if caps.Events {
					cam.Capabilities = append(cam.Capabilities, "events")
				}
				if caps.Imaging {
					cam.Capabilities = append(cam.Capabilities, "imaging")
				}
				if caps.Media {
					cam.Capabilities = append(cam.Capabilities, "media")
				}
				if caps.PTZ {
					cam.Capabilities = append(cam.Capabilities, "ptz")
				}
				if caps.Recording {
					cam.Capabilities = append(cam.Capabilities, "recording")
				}
			} else {
				s.logger.Debug("GetCapabilities failed", "device", deviceURL, "error", err)
			}

			if hostname, err := onvif.GetHostname(ctx, client, deviceURL); err == nil {
				cam.Hostname = hostname
			}

			cameras = append(cameras, cam)
		}
	}

	return cameras, nil
}

func (s *DiscoveryService) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	s.scanning = true
	s.scanError = ""
	s.results = nil
	s.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.config.ScanTimeout+2*time.Second)
		defer cancel()

		cameras, err := s.sendProbe(ctx)

		s.mu.Lock()
		s.scanning = false
		if err != nil {
			s.scanError = err.Error()
			s.logger.Error("Scan failed", "error", err)
		} else {
			s.results = cameras
			s.logger.Info("Discovery scan complete", "cameras_found", len(cameras))
		}
		s.mu.Unlock()

		if err == nil && s.natsConn != nil {
			for _, cam := range cameras {
				data, err := json.Marshal(cam)
				if err != nil {
					s.logger.Warn("Failed to marshal camera for NATS", "error", err)
					continue
				}
				if err := s.natsConn.Publish("cameras.discovered", data); err != nil {
					s.logger.Warn("Failed to publish camera to NATS", "error", err)
				}
			}
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "scanning"})
}

func (s *DiscoveryService) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	status := scanStatus{
		Scanning: s.scanning,
		Count:    len(s.results),
		Error:    s.scanError,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *DiscoveryService) handleResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	if s.results == nil {
		w.Write([]byte(`[]`))
		return
	}
	json.NewEncoder(w).Encode(s.results)
}



func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := common.InitTelemetry("discovery"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultDiscoveryConfig()

	common.StartMetricsServer(config.MetricsPort)
	common.StartResourceMonitor(ctx)

	service, err := NewDiscoveryService(config, logger)
	if err != nil {
		logger.Error("Failed to initialize discovery service", "error", err)
		os.Exit(1)
	}

	if err := service.Start(); err != nil {
		logger.Error("Failed to start discovery service", "error", err)
		os.Exit(1)
	}

	<-ctx.Done()
	logger.Info("Shutting down Discovery Service...")

	shutdownCtx, cancel := context.WithTimeout(ctx, config.GracefulTimeout)
	defer cancel()

	if err := service.Shutdown(shutdownCtx); err != nil {
		logger.Error("Error during shutdown", "error", err)
	}
}
