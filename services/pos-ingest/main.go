package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

type POSItem struct {
	SKU         string  `json:"sku"`
	Description string  `json:"description"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	Total       float64 `json:"total"`
}

type POSTransaction struct {
	ID            string    `json:"id"`
	CameraID      string    `json:"camera_id"`
	StoreID       string    `json:"store_id"`
	RegisterID    string    `json:"register_id"`
	TransactionID string    `json:"transaction_id"`
	Timestamp     time.Time `json:"timestamp"`
	Items         []POSItem `json:"items"`
	Subtotal      float64   `json:"subtotal"`
	Tax           float64   `json:"tax"`
	Total         float64   `json:"total"`
	TenderType    string    `json:"tender_type"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := common.InitTelemetry("pos-ingest"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	common.StartMetricsServer(common.GetEnv("METRICS_ADDR", ":2112"))
	common.StartResourceMonitor(ctx)

	nc, err := nats.Connect(common.GetEnv("NATS_URL", "nats://localhost:4222"), append(common.NATSTLSOptions(),
		nats.RetryOnFailedConnect(true), nats.MaxReconnects(-1), nats.ReconnectWait(2*time.Second))...)
	if err != nil {
		logger.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()

	mux := http.NewServeMux()
	healthHandler := common.NewHealthHandler()
	healthHandler.AddNATSChecker(nc, "nats")
	mux.HandleFunc("/health", healthHandler.Liveness)
	mux.HandleFunc("/ready", healthHandler.Readiness)
	mux.Handle("/api/pos/transaction", common.JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var tx POSTransaction
		if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
			jsonError(w, "invalid transaction", http.StatusBadRequest)
			return
		}

		if tx.ID == "" {
			tx.ID = uuid.New().String()
		}
		if tx.Timestamp.IsZero() {
			tx.Timestamp = time.Now().UTC()
		}

		data, err := json.Marshal(tx)
		if err != nil {
			jsonError(w, "failed to marshal transaction", http.StatusInternalServerError)
			return
		}
		if err := nc.Publish("pos.transaction", data); err != nil {
			logger.Error("Failed to publish POS transaction", "error", err)
			jsonError(w, "failed to publish transaction", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted", "id": tx.ID})
	}))

	server := &http.Server{
		Addr:         ":8096",
		Handler:      common.RecoveryMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("POS ingest listening", "addr", ":8096")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server error", "error", err)
		}
	}()

	<-ctx.Done()
	logger.Info("Shutting down POS ingest...")
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
