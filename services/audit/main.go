package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
)

type AuditEntry struct {
	ID           string `json:"id"`
	Timestamp    string `json:"timestamp"`
	Action       string `json:"action"`
	Actor        string `json:"actor"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Details      string `json:"details"`
	PreviousHash string `json:"previous_hash"`
	Hash         string `json:"hash"`
}

type AuditService struct {
	mu      sync.RWMutex
	entries []AuditEntry
	db      *sql.DB
	nc      *nats.Conn
	sub     *nats.Subscription
	logger  *slog.Logger
	port    string
	natsURL string
	dbURL   string
}

func NewAuditService(logger *slog.Logger) (*AuditService, error) {
	natsURL := common.GetEnv("NATS_URL", "nats://localhost:4222")
	port := common.GetEnv("AUDIT_PORT", ":8093")
	dbURL := os.Getenv("DATABASE_URL")

	nc, err := nats.Connect(natsURL, append(common.NATSTLSOptions(),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	s := &AuditService{
		logger:  logger,
		port:    port,
		natsURL: natsURL,
		dbURL:   dbURL,
		nc:      nc,
	}

	if dbURL != "" {
		db, err := sql.Open("postgres", dbURL)
		if err != nil {
			logger.Warn("Failed to open database, falling back to in-memory", "error", err)
		} else {
			db.SetMaxOpenConns(10)
			db.SetMaxIdleConns(5)
			db.SetConnMaxLifetime(5 * time.Minute)
			if err := db.Ping(); err != nil {
				logger.Warn("Failed to ping database, falling back to in-memory", "error", err)
			} else {
				s.db = db
				logger.Info("Connected to database for audit persistence")

				s.runMigrations()
			}
		}
	} else {
		logger.Info("DATABASE_URL not set, audit persistence will be in-memory only")
	}

	return s, nil
}

func (s *AuditService) runMigrations() {
	if s.db == nil {
		return
	}
	dir := common.GetEnv("MIGRATIONS_DIR", "/migrations")
	dbx := sqlx.NewDb(s.db, "postgres")
	migrator := common.NewMigrator(dbx, dir, s.logger)
	if err := migrator.Run(); err != nil {
		s.logger.Error("Failed to run migrations", "error", err)
	}
}

func (s *AuditService) Start() error {
	var err error
	s.sub, err = s.nc.QueueSubscribe("audit.log", "audit", s.handleAuditMessage)
	if err != nil {
		return fmt.Errorf("failed to subscribe to audit.log: %w", err)
	}
	s.logger.Info("Audit service started", "port", s.port)
	return nil
}

func (s *AuditService) computeHash(prevHash, timestamp, action, actor, resourceType, resourceID, details string) string {
	h := sha256.New()
	h.Write([]byte(prevHash + timestamp + action + actor + resourceType + resourceID + details))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (s *AuditService) logEntry(entry *AuditEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range s.entries {
		if e.ID == entry.ID {
			return
		}
	}

	prevHash := "0000000000000000000000000000000000000000000000000000000000000000"
	if len(s.entries) > 0 {
		prevHash = s.entries[len(s.entries)-1].Hash
	}
	entry.PreviousHash = prevHash
	entry.Hash = s.computeHash(prevHash, entry.Timestamp, entry.Action, entry.Actor, entry.ResourceType, entry.ResourceID, entry.Details)

	s.entries = append(s.entries, *entry)
	s.logger.Info("Audit entry recorded", "id", entry.ID, "action", entry.Action, "actor", entry.Actor)

	s.persistEntry(entry)
}

func (s *AuditService) persistEntry(entry *AuditEntry) {
	if s.db == nil {
		return
	}
	_, err := s.db.Exec(
		`INSERT INTO audit_logs (id, action, resource_type, created_at, actor, resource_id_str, details_text, previous_hash, hash)
		 VALUES ($1, $2, $3, $4::TIMESTAMPTZ, $5, $6, $7, $8, $9)`,
		entry.ID, entry.Action, entry.ResourceType, entry.Timestamp,
		entry.Actor, entry.ResourceID, entry.Details,
		entry.PreviousHash, entry.Hash,
	)
	if err != nil {
		s.logger.Error("Failed to persist audit entry to database", "error", err, "id", entry.ID)
	}
}

func (s *AuditService) handleAuditMessage(msg *nats.Msg) {
	var entry AuditEntry
	if err := json.Unmarshal(msg.Data, &entry); err != nil {
		s.logger.Error("Failed to unmarshal audit entry", "error", err)
		return
	}

	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	s.logEntry(&entry)
}

func (s *AuditService) handleCreateEntry(w http.ResponseWriter, r *http.Request) {
	var entry AuditEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if entry.Action == "" || entry.Actor == "" || entry.ResourceType == "" || entry.ResourceID == "" {
		jsonError(w, "action, actor, resource_type, and resource_id are required", http.StatusBadRequest)
		return
	}

	entry.ID = uuid.New().String()
	entry.Timestamp = time.Now().UTC().Format(time.RFC3339)

	s.logEntry(&entry)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(entry)
}

func (s *AuditService) handleGetChain(w http.ResponseWriter, r *http.Request) {
	chain := s.getEntries()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chain)
}

func (s *AuditService) getEntries() []AuditEntry {
	if s.db != nil {
		return s.getEntriesFromDB()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	chain := make([]AuditEntry, len(s.entries))
	copy(chain, s.entries)
	return chain
}

func (s *AuditService) getEntriesFromDB() []AuditEntry {
	rows, err := s.db.Query(
		`SELECT id, action, resource_type, created_at::TEXT,
		        COALESCE(actor, ''), COALESCE(resource_id_str, ''),
		        COALESCE(details_text, ''), COALESCE(previous_hash, ''),
		        COALESCE(hash, '')
		 FROM audit_logs
		 ORDER BY created_at ASC, id ASC`,
	)
	if err != nil {
		s.logger.Error("Failed to query audit entries from database", "error", err)
		s.mu.RLock()
		defer s.mu.RUnlock()
		chain := make([]AuditEntry, len(s.entries))
		copy(chain, s.entries)
		return chain
	}
	defer rows.Close()

	var chain []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.Action, &e.ResourceType, &e.Timestamp,
			&e.Actor, &e.ResourceID, &e.Details, &e.PreviousHash, &e.Hash); err != nil {
			s.logger.Error("Failed to scan audit entry from database", "error", err)
			continue
		}
		chain = append(chain, e)
	}
	return chain
}

func (s *AuditService) handleVerify(w http.ResponseWriter, r *http.Request) {
	entries := s.getEntries()

	result := struct {
		Valid     bool   `json:"valid"`
		Count     int    `json:"count"`
		FirstHash string `json:"first_hash"`
		LastHash  string `json:"last_hash"`
	}{
		Count: len(entries),
	}

	if len(entries) == 0 {
		result.Valid = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
		return
	}

	result.FirstHash = entries[0].Hash
	result.LastHash = entries[len(entries)-1].Hash
	result.Valid = true

	for i, entry := range entries {
		expectedPrevHash := "0000000000000000000000000000000000000000000000000000000000000000"
		if i > 0 {
			expectedPrevHash = entries[i-1].Hash
		}
		if entry.PreviousHash != expectedPrevHash {
			result.Valid = false
			break
		}
		expectedHash := s.computeHash(entry.PreviousHash, entry.Timestamp, entry.Action, entry.Actor, entry.ResourceType, entry.ResourceID, entry.Details)
		if entry.Hash != expectedHash {
			result.Valid = false
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *AuditService) Close() error {
	var errs []error
	if s.sub != nil {
		if err := s.sub.Unsubscribe(); err != nil {
			errs = append(errs, fmt.Errorf("failed to unsubscribe: %w", err))
		}
	}
	if s.nc != nil {
		s.nc.Close()
	}
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close database: %w", err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errs)
	}
	return nil
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	common.CheckJWTSecret()

	if err := common.InitTelemetry("audit"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	common.StartMetricsServer(common.GetEnv("METRICS_ADDR", ":2112"))
	common.StartResourceMonitor(ctx)

	service, err := NewAuditService(logger)
	if err != nil {
		logger.Error("Failed to create audit service", "error", err)
		os.Exit(1)
	}
	defer service.Close()

	if err := service.Start(); err != nil {
		logger.Error("Failed to start audit service", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	healthHandler := common.NewHealthHandler()
	healthHandler.AddNATSChecker(service.nc, "nats")
	if service.db != nil {
		healthHandler.AddDBChecker(service.db, "postgres")
	}
	mux.HandleFunc("/health", healthHandler.Liveness)
	mux.HandleFunc("/ready", healthHandler.Readiness)
	mux.Handle("/api/audit/log", common.JWTAuthMiddleware(service.handleCreateEntry))
	mux.Handle("/api/audit/chain", common.JWTAuthMiddleware(service.handleGetChain))
	mux.Handle("/api/audit/verify", common.JWTAuthMiddleware(service.handleVerify))

	server := &http.Server{
		Addr:         service.port,
		Handler:      common.RecoveryMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("Audit service listening", "addr", service.port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
		}
	}()

	<-ctx.Done()
	logger.Info("Shutting down audit service...")

	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}
