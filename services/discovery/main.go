package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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

type DiscoveryService struct {
	config        *DiscoveryConfig
	logger        *slog.Logger
	db            *sqlx.DB
	store         *ResultStore
	orchestrator  *ScanOrchestrator
	scheduler     *Scheduler
	scanners      map[string]Scanner
	natsConn      *nats.Conn
	server        *http.Server
	healthHandler *common.HealthHandler
}

func NewDiscoveryService(config *DiscoveryConfig, logger *slog.Logger) (*DiscoveryService, error) {
	s := &DiscoveryService{
		config:        config,
		logger:        logger,
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
			s.store = NewResultStore(s.db, logger)
			s.scanners = map[string]Scanner{
				"ws-discovery": NewWSDiscoveryScanner(logger),
				"ip-range":     NewIPRangeScanner(logger),
				"mdns":         NewMDNSScanner(logger),
				"manual":       NewManualIPScanner(logger),
			}
			s.orchestrator = NewScanOrchestrator(s.store, s.scanners, logger)
			s.scheduler = NewScheduler(s.db, s.orchestrator, logger)
			s.healthHandler.AddDBChecker(db.DB, "postgres")
			logger.Info("Connected to database")
		}
	}

	return s, nil
}

func (s *DiscoveryService) Start() error {
	mux := http.NewServeMux()

	// DB-backed endpoints only available when database is connected
	if s.orchestrator != nil && s.store != nil {
		scanHandler := NewScanHandler(s.orchestrator, s.store, s.logger)

		mux.HandleFunc("/discovery/scans", common.JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				scanHandler.handleCreateScan(w, r)
			case http.MethodGet:
				scanHandler.handleListScans(w, r)
			default:
				jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		}))
		mux.HandleFunc("/discovery/scans/{id}", common.JWTAuthMiddleware(scanHandler.handleGetScan))
		mux.HandleFunc("/discovery/scans/{id}/cancel", common.JWTAuthMiddleware(scanHandler.handleCancelScan))
		mux.HandleFunc("/discovery/scans/{id}/results", common.JWTAuthMiddleware(scanHandler.handleGetResults))
		mux.HandleFunc("/discovery/scans/{id}/import", common.JWTAuthMiddleware(scanHandler.handleImport))
	}

	mux.HandleFunc("/discovery/credentials/test", common.JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		(&ScanHandler{logger: s.logger}).handleTestCredentials(w, r)
	}))
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

	if s.scheduler != nil {
		go s.scheduler.Start(context.Background())
	}

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
