package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"

	"github.com/dam-vms/dam/pkg/common"
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

		scanner := NewWSDiscoveryScanner(s.logger)
		resultCh, err := scanner.Scan(ctx, "", nil, ScanOptions{Timeout: s.config.ScanTimeout})
		if err != nil {
			s.mu.Lock()
			s.scanning = false
			s.scanError = err.Error()
			s.logger.Error("Scan failed", "error", err)
			s.mu.Unlock()
			return
		}

		var cameras []discoveredCamera
		for res := range resultCh {
			if res.Error != nil {
				continue
			}
			cam := discoveredCamera{
				Address:      res.IP,
				Manufacturer: res.Manufacturer,
				Model:        res.Model,
				Firmware:     res.Firmware,
				SerialNumber: res.SerialNumber,
				Hostname:     res.Hostname,
				XAddrs:       res.XAddr,
			}
			var caps []string
			for k := range res.Capabilities {
				caps = append(caps, k)
			}
			cam.Capabilities = caps
			cameras = append(cameras, cam)
		}

		s.mu.Lock()
		s.scanning = false
		s.results = cameras
		s.logger.Info("Discovery scan complete", "cameras_found", len(cameras))
		s.mu.Unlock()

		if s.natsConn != nil {
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
