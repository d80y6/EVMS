package common

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

type HealthStatus string

const (
	HealthOK       HealthStatus = "ok"
	HealthDegraded HealthStatus = "degraded"
	HealthDown     HealthStatus = "down"
)

type CheckResult struct {
	Name   string       `json:"name"`
	Status HealthStatus `json:"status"`
	Error  string       `json:"error,omitempty"`
	Latency string      `json:"latency,omitempty"`
}

type HealthResponse struct {
	Status    HealthStatus   `json:"status"`
	Timestamp time.Time      `json:"timestamp"`
	Checks    []CheckResult  `json:"checks"`
}

type HealthHandler struct {
	mu       sync.RWMutex
	checkers map[string]HealthChecker
}

type HealthChecker func(ctx context.Context) CheckResult

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{
		checkers: make(map[string]HealthChecker),
	}
}

func (h *HealthHandler) AddChecker(name string, fn HealthChecker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checkers[name] = fn
}

func (h *HealthHandler) AddDBChecker(db *sql.DB, name string) {
	if db == nil {
		return
	}
	h.AddChecker(name, func(ctx context.Context) CheckResult {
		start := time.Now()
		err := db.PingContext(ctx)
		latency := time.Since(start).String()
		if err != nil {
			return CheckResult{Name: name, Status: HealthDown, Error: err.Error(), Latency: latency}
		}
		return CheckResult{Name: name, Status: HealthOK, Latency: latency}
	})
}

func (h *HealthHandler) AddNATSChecker(nc *nats.Conn, name string) {
	if nc == nil {
		return
	}
	h.AddChecker(name, func(ctx context.Context) CheckResult {
		start := time.Now()
		latency := time.Since(start).String()
		if !nc.IsConnected() {
			return CheckResult{Name: name, Status: HealthDown, Error: "not connected", Latency: latency}
		}
		if nc.Status() != nats.CONNECTED {
			return CheckResult{Name: name, Status: HealthDown, Error: nc.Status().String(), Latency: latency}
		}
		return CheckResult{Name: name, Status: HealthOK, Latency: latency}
	})
}

func (h *HealthHandler) runChecks(ctx context.Context) ([]CheckResult, HealthStatus) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	overall := HealthOK
	results := make([]CheckResult, 0, len(h.checkers))

	for name, checker := range h.checkers {
		result := checker(ctx)
		if result.Status != HealthOK {
			overall = HealthDegraded
		}
		result.Name = name
		results = append(results, result)
	}

	return results, overall
}

func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Health-Check", "liveness")

	results, status := h.runChecks(r.Context())
	resp := HealthResponse{
		Status:    status,
		Timestamp: time.Now().UTC(),
		Checks:    results,
	}

	if status == HealthDown {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	json.NewEncoder(w).Encode(resp)
}

func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Health-Check", "readiness")

	results, status := h.runChecks(r.Context())
	resp := HealthResponse{
		Status:    status,
		Timestamp: time.Now().UTC(),
		Checks:    results,
	}

	if status != HealthOK {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	json.NewEncoder(w).Encode(resp)
}

func HealthCheckMiddleware(handler *HealthHandler, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/ready" {
			if r.URL.Path == "/ready" {
				handler.Readiness(w, r)
			} else {
				handler.Liveness(w, r)
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}
