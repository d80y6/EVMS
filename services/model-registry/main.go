package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"

	"github.com/dam-vms/dam/pkg/common"
)

type ModelConfig struct {
	NATSURL   string
	DBURL     string
	Port      string
	MetricsAddr string
}

func DefaultModelConfig() *ModelConfig {
	return &ModelConfig{
		NATSURL:     common.GetEnv("NATS_URL", "nats://nats:4222"),
		DBURL:       os.Getenv("DB_URL"),
		Port:        common.GetEnv("MODEL_REGISTRY_PORT", ":8098"),
		MetricsAddr: common.GetEnv("METRICS_ADDR", ":2112"),
	}
}

type Model struct {
	ID            string          `json:"id" db:"id"`
	Name          string          `json:"name" db:"name"`
	Version       int             `json:"version" db:"version"`
	Status        string          `json:"status" db:"status"`
	ModelPath     string          `json:"model_path" db:"model_path"`
	Metrics       json.RawMessage `json:"metrics" db:"metrics"`
	CanaryPercent int             `json:"canary_percent" db:"canary_percent"`
	CreatedAt     time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at" db:"updated_at"`
}

type ModelRegistry struct {
	config    *ModelConfig
	nc        *nats.Conn
	db        *sqlx.DB
	logger    *slog.Logger
	httpSrv   *http.Server
}

func NewModelRegistry(config *ModelConfig, logger *slog.Logger) (*ModelRegistry, error) {
	var db *sqlx.DB
	if config.DBURL != "" {
		cb := common.NewDBCircuitBreaker("model-registry")
		var err error
		db, err = common.ConnectDBWithCircuitBreaker(context.Background(), "postgres", config.DBURL, cb)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to database: %w", err)
		}
	}

	nc, err := nats.Connect(config.NATSURL, append(common.NATSTLSOptions(),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	h := common.NewHealthHandler()
	if nc != nil {
		h.AddNATSChecker(nc, "nats")
	}
	if db != nil {
		h.AddDBChecker(db.DB, "postgres")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.Liveness)
	mux.HandleFunc("/ready", h.Readiness)

	mux.Handle("/api/models", common.JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listModels(db, w, r)
		case http.MethodPost:
			createModel(db, w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/models/", common.JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/models/")
		parts := strings.Split(strings.TrimRight(id, "/"), "/")

		if len(parts) == 0 || parts[0] == "" {
			http.Error(w, "model id required", http.StatusBadRequest)
			return
		}

		modelID := parts[0]
		action := ""
		if len(parts) > 1 {
			action = parts[1]
		}

		switch {
		case action == "" && r.Method == http.MethodGet:
			getModel(db, w, r, modelID)
		case action == "activate" && r.Method == http.MethodPost:
			activateVersion(db, w, r, modelID)
		case action == "canary" && r.Method == http.MethodPost:
			deployCanary(db, w, r, modelID)
		case action == "promote" && r.Method == http.MethodPost:
			promoteCanary(db, nc, w, r, modelID)
		case action == "rollback" && r.Method == http.MethodPost:
			rollback(db, nc, w, r, modelID)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))

	return &ModelRegistry{
		config:  config,
		nc:      nc,
		db:      db,
		logger:  logger,
		httpSrv: &http.Server{Addr: config.Port, Handler: common.RecoveryMiddleware(mux)},
	}, nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func createModel(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	if db == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
		return
	}

	var req struct {
		Name      string          `json:"name"`
		ModelPath string          `json:"model_path"`
		Metrics   json.RawMessage `json:"metrics,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	id := uuid.New().String()
	now := time.Now().UTC()

	var existingVersion int
	err := db.Get(&existingVersion, "SELECT COALESCE(MAX(version), 0) FROM ai_models WHERE name = $1", req.Name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get next version"})
		return
	}
	nextVersion := existingVersion + 1

	if req.Metrics == nil {
		req.Metrics = json.RawMessage("{}")
	}

	_, err = db.Exec(
		`INSERT INTO ai_models (id, name, version, status, model_path, metrics, canary_percent, created_at, updated_at)
		 VALUES ($1, $2, $3, 'inactive', $4, $5, 0, $6, $6)`,
		id, req.Name, nextVersion, req.ModelPath, req.Metrics, now,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create model"})
		return
	}

	model := Model{
		ID:            id,
		Name:          req.Name,
		Version:       nextVersion,
		Status:        "inactive",
		ModelPath:     req.ModelPath,
		Metrics:       req.Metrics,
		CanaryPercent: 0,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	writeJSON(w, http.StatusCreated, model)
}

func listModels(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	if db == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
		return
	}

	var models []Model
	query := `SELECT id, name, version, status, model_path, metrics, canary_percent, created_at, updated_at
		FROM ai_models ORDER BY name, version DESC`

	if err := db.Select(&models, query); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list models"})
		return
	}
	if models == nil {
		models = []Model{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"models": models})
}

func getModel(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	if db == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
		return
	}

	var model Model
	err := db.Get(&model,
		`SELECT id, name, version, status, model_path, metrics, canary_percent, created_at, updated_at
		 FROM ai_models WHERE id = $1`, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "model not found"})
		return
	}

	writeJSON(w, http.StatusOK, model)
}

func activateVersion(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	if db == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
		return
	}

	tx, err := db.Beginx()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to begin transaction"})
		return
	}
	defer tx.Rollback()

	var model Model
	err = tx.Get(&model,
		`SELECT id, name, version, status, model_path, metrics, canary_percent, created_at, updated_at
		 FROM ai_models WHERE id = $1 FOR UPDATE`, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "model not found"})
		return
	}

	if model.Status != "inactive" && model.Status != "archived" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "only inactive or archived models can be activated"})
		return
	}

	_, err = tx.Exec(`UPDATE ai_models SET status = 'inactive', updated_at = NOW()
		WHERE name = $1 AND status = 'active'`, model.Name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to deactivate current version"})
		return
	}

	_, err = tx.Exec(`UPDATE ai_models SET status = 'active', canary_percent = 0, updated_at = NOW()
		WHERE id = $1`, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to activate model"})
		return
	}

	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to commit transaction"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "activated", "model_id": id})
}

func deployCanary(db *sqlx.DB, w http.ResponseWriter, r *http.Request, id string) {
	if db == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
		return
	}

	var req struct {
		Percent int `json:"percent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Percent < 1 || req.Percent > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "percent must be between 1 and 100"})
		return
	}

	var model Model
	err := db.Get(&model,
		`SELECT id, name, version, status, model_path, metrics, canary_percent, created_at, updated_at
		 FROM ai_models WHERE id = $1`, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "model not found"})
		return
	}

	if model.Status != "inactive" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "only inactive models can be deployed as canary"})
		return
	}

	_, err = db.Exec(`UPDATE ai_models SET status = 'canary', canary_percent = $1, updated_at = NOW()
		WHERE id = $2`, req.Percent, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to deploy canary"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":         "canary_deployed",
		"model_id":       id,
		"canary_percent": req.Percent,
	})
}

func promoteCanary(db *sqlx.DB, nc *nats.Conn, w http.ResponseWriter, r *http.Request, id string) {
	if db == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
		return
	}

	tx, err := db.Beginx()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to begin transaction"})
		return
	}
	defer tx.Rollback()

	var model Model
	err = tx.Get(&model,
		`SELECT id, name, version, status, model_path, metrics, canary_percent, created_at, updated_at
		 FROM ai_models WHERE id = $1 FOR UPDATE`, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "model not found"})
		return
	}

	if model.Status != "canary" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "only canary models can be promoted"})
		return
	}

	_, err = tx.Exec(`UPDATE ai_models SET status = 'inactive', updated_at = NOW()
		WHERE name = $1 AND status = 'active'`, model.Name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to deactivate current version"})
		return
	}

	_, err = tx.Exec(`UPDATE ai_models SET status = 'active', canary_percent = 0, updated_at = NOW()
		WHERE id = $1`, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to promote canary"})
		return
	}

	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to commit transaction"})
		return
	}

	if nc != nil {
		event := map[string]interface{}{
			"type":       "model.promoted",
			"model_id":   model.ID,
			"name":       model.Name,
			"version":    model.Version,
			"model_path": model.ModelPath,
			"timestamp":  time.Now().UTC(),
		}
		data, _ := json.Marshal(event)
		nc.Publish("models.events", data)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "promoted", "model_id": id})
}

func rollback(db *sqlx.DB, nc *nats.Conn, w http.ResponseWriter, r *http.Request, id string) {
	if db == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
		return
	}

	tx, err := db.Beginx()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to begin transaction"})
		return
	}
	defer tx.Rollback()

	var model Model
	err = tx.Get(&model,
		`SELECT id, name, version, status, model_path, metrics, canary_percent, created_at, updated_at
		 FROM ai_models WHERE id = $1 FOR UPDATE`, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "model not found"})
		return
	}

	if model.Status == "inactive" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "inactive models cannot be rolled back"})
		return
	}

	var prevModel Model
	err = tx.Get(&prevModel,
		`SELECT id, name, version, status, model_path, metrics, canary_percent, created_at, updated_at
		 FROM ai_models WHERE name = $1 AND status IN ('active', 'canary') AND id != $2
		 ORDER BY version DESC LIMIT 1`, model.Name, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no previous version to rollback to"})
		return
	}

	previousStatus := model.Status
	_, err = tx.Exec(`UPDATE ai_models SET status = 'inactive', canary_percent = 0, updated_at = NOW()
		WHERE id = $1`, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to deactivate current model"})
		return
	}

	newStatus := "active"
	if previousStatus == "canary" {
		newStatus = "inactive"
	}

	_, err = tx.Exec(`UPDATE ai_models SET status = $1, canary_percent = 0, updated_at = NOW()
		WHERE id = $2`, newStatus, prevModel.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to restore previous version"})
		return
	}

	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to commit transaction"})
		return
	}

	if nc != nil {
		event := map[string]interface{}{
			"type":          "model.rolled_back",
			"model_id":      model.ID,
			"name":          model.Name,
			"version":       model.Version,
			"previous_id":   prevModel.ID,
			"previous_version": prevModel.Version,
			"timestamp":     time.Now().UTC(),
		}
		data, _ := json.Marshal(event)
		nc.Publish("models.events", data)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":          "rolled_back",
		"rolled_back_id":  model.ID,
		"restored_id":     prevModel.ID,
		"restored_version": prevModel.Version,
	})
}

func (r *ModelRegistry) Close() error {
	var errs []error
	if r.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.httpSrv.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown http server: %w", err))
		}
	}
	if r.nc != nil {
		r.nc.Close()
	}
	if r.db != nil {
		if err := r.db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close database: %w", err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errs)
	}
	return nil
}

func main() {
	logger := common.NewLogger("model-registry")
	slog.SetDefault(logger)

	common.CheckJWTSecret()

	if err := common.InitTelemetry("model-registry"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultModelConfig()

	common.StartMetricsServer(config.MetricsAddr)
	common.StartResourceMonitor(ctx)

	registry, err := NewModelRegistry(config, logger)
	if err != nil {
		logger.Error("Failed to initialize model registry", "error", err)
		os.Exit(1)
	}
	defer registry.Close()

	go func() {
		logger.Info("Model Registry listening", "port", config.Port)
		if err := registry.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("Shutting down Model Registry...")
}
