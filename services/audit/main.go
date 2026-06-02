package main

import (
	"context"
	"crypto/sha256"
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
	nc      *nats.Conn
	sub     *nats.Subscription
	logger  *slog.Logger
	port    string
	natsURL string
}

func NewAuditService(logger *slog.Logger) (*AuditService, error) {
	natsURL := common.GetEnv("NATS_URL", "nats://localhost:4222")
	port := common.GetEnv("AUDIT_PORT", "8093")

	nc, err := nats.Connect(natsURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	return &AuditService{
		logger:  logger,
		port:    port,
		natsURL: natsURL,
		nc:      nc,
	}, nil
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

func (s *AuditService) handleAuditMessage(msg *nats.Msg) {
	var entry AuditEntry
	if err := json.Unmarshal(msg.Data, &entry); err != nil {
		s.logger.Error("Failed to unmarshal audit entry", "error", err)
		return
	}

	entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}

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

	s.entries = append(s.entries, entry)
	s.logger.Info("Audit entry recorded", "id", entry.ID, "action", entry.Action, "actor", entry.Actor)
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

	data, err := json.Marshal(entry)
	if err != nil {
		jsonError(w, "failed to marshal entry", http.StatusInternalServerError)
		return
	}

	if err := s.nc.Publish("audit.log", data); err != nil {
		jsonError(w, "failed to publish audit entry", http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	prevHash := ""
	if len(s.entries) == 0 {
		prevHash = "0000000000000000000000000000000000000000000000000000000000000000"
	} else {
		prevHash = s.entries[len(s.entries)-1].Hash
	}
	entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	entry.PreviousHash = prevHash
	entry.Hash = s.computeHash(prevHash, entry.Timestamp, entry.Action, entry.Actor, entry.ResourceType, entry.ResourceID, entry.Details)
	s.entries = append(s.entries, entry)
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(entry)
}

func (s *AuditService) handleGetChain(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	chain := make([]AuditEntry, len(s.entries))
	copy(chain, s.entries)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chain)
}

func (s *AuditService) handleVerify(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := struct {
		Valid     bool   `json:"valid"`
		Count     int    `json:"count"`
		FirstHash string `json:"first_hash"`
		LastHash  string `json:"last_hash"`
	}{
		Count: len(s.entries),
	}

	if len(s.entries) == 0 {
		result.Valid = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
		return
	}

	result.FirstHash = s.entries[0].Hash
	result.LastHash = s.entries[len(s.entries)-1].Hash
	result.Valid = true

	for i, entry := range s.entries {
		expectedPrevHash := "0000000000000000000000000000000000000000000000000000000000000000"
		if i > 0 {
			expectedPrevHash = s.entries[i-1].Hash
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
	mux.HandleFunc("/health", healthHandler.Liveness)
	mux.HandleFunc("/ready", healthHandler.Readiness)
	mux.HandleFunc("/api/audit/log", service.handleCreateEntry)
	mux.HandleFunc("/api/audit/chain", service.handleGetChain)
	mux.HandleFunc("/api/audit/verify", service.handleVerify)

	server := &http.Server{
		Addr:         ":" + service.port,
		Handler:      common.RecoveryMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("Audit service listening", "addr", ":"+service.port)
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
