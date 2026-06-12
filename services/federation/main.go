package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"github.com/dam-vms/dam/pkg/common"
)

type FederatedSite struct {
	ID        string `json:"id" db:"id"`
	Name      string `json:"name" db:"name"`
	URL       string `json:"url" db:"url"`
	APIKey    string `json:"api_key,omitempty" db:"api_key"`
	Status    string `json:"status" db:"status"`
	Latency   int    `json:"latency_ms" db:"-"`
	LastSeen  string `json:"last_seen" db:"-"`
	CreatedAt string `json:"created_at" db:"created_at"`
	UpdatedAt string `json:"updated_at" db:"updated_at"`
}

type FederationConfig struct {
	Port  string
	DBURL string
}

func DefaultFederationConfig() *FederationConfig {
	return &FederationConfig{
		Port:  common.GetEnv("FEDERATION_PORT", ":8099"),
		DBURL: common.GetEnv("DB_URL", ""),
	}
}

type FederationService struct {
	config        *FederationConfig
	logger        *slog.Logger
	db            *sqlx.DB
	server        *http.Server
	httpClient    *http.Client
	healthHandler *common.HealthHandler
}

func NewFederationService(config *FederationConfig, logger *slog.Logger) (*FederationService, error) {
	if logger == nil {
		logger = slog.Default()
	}

	var db *sqlx.DB
	if config.DBURL != "" {
		var err error
		db, err = sqlx.Connect("postgres", config.DBURL)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to database: %w", err)
		}
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)
	}

	h := common.NewHealthHandler()
	if db != nil {
		h.AddDBChecker(db.DB, "postgres")
	}

	return &FederationService{
		config: config,
		logger: logger,
		db:     db,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        50,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		healthHandler: h,
	}, nil
}

func (s *FederationService) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/federation/sites", s.handleSites)
	mux.HandleFunc("/api/federation/sites/", s.handleSiteByID)
	mux.HandleFunc("/api/federation/search", s.handleSearch)
	mux.HandleFunc("/api/federation/playback/", s.handlePlaybackProxy)
	mux.HandleFunc("/health", s.healthHandler.Liveness)
	mux.HandleFunc("/ready", s.healthHandler.Readiness)

	s.server = &http.Server{
		Addr:         s.config.Port,
		Handler:      common.RecoveryMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("Federation Service started", "address", s.config.Port)
	return s.server.ListenAndServe()
}

func (s *FederationService) Shutdown(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *FederationService) handleSites(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listSites(w, r)
	case http.MethodPost:
		s.registerSite(w, r)
	default:
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *FederationService) listSites(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var sites []FederatedSite
	err := s.db.SelectContext(ctx, &sites, "SELECT id, name, url, status, created_at, updated_at FROM federated_sites ORDER BY name")
	if err != nil {
		s.logger.Error("Failed to list federated sites", "error", err)
		jsonError(w, "failed to list sites", http.StatusInternalServerError)
		return
	}

	if sites == nil {
		sites = []FederatedSite{}
	}

	now := time.Now()
	for i := range sites {
		sites[i].Latency = s.measureLatency(sites[i].URL)
		sites[i].LastSeen = now.Format(time.RFC3339)
	}

	jsonOK(w, map[string]interface{}{"sites": sites})
}

func (s *FederationService) registerSite(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	var req struct {
		Name   string `json:"name"`
		URL    string `json:"url"`
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.URL == "" {
		jsonError(w, "name and url are required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	id := uuid.New().String()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO federated_sites (id, name, url, api_key, status) VALUES ($1, $2, $3, $4, 'inactive')`,
		id, req.Name, req.URL, req.APIKey)
	if err != nil {
		s.logger.Error("Failed to register site", "error", err)
		jsonError(w, "failed to register site", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"site": map[string]string{
			"id":   id,
			"name": req.Name,
			"url":  req.URL,
		},
	})
}

func (s *FederationService) handleSiteByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/federation/sites/")
	if id == "" {
		jsonError(w, "site id required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getSite(w, r, id)
	case http.MethodPut:
		s.updateSite(w, r, id)
	case http.MethodDelete:
		s.deleteSite(w, r, id)
	default:
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *FederationService) getSite(w http.ResponseWriter, r *http.Request, id string) {
	if s.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var site FederatedSite
	err := s.db.GetContext(ctx, &site, "SELECT id, name, url, status, created_at, updated_at FROM federated_sites WHERE id = $1", id)
	if err != nil {
		jsonError(w, "site not found", http.StatusNotFound)
		return
	}

	site.Latency = s.measureLatency(site.URL)
	site.LastSeen = time.Now().Format(time.RFC3339)

	jsonOK(w, map[string]interface{}{"site": site})
}

func (s *FederationService) updateSite(w http.ResponseWriter, r *http.Request, id string) {
	if s.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	var req struct {
		Name   *string `json:"name"`
		URL    *string `json:"url"`
		APIKey *string `json:"api_key"`
		Status *string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx,
		`UPDATE federated_sites SET
			name = COALESCE($1, name),
			url = COALESCE($2, url),
			api_key = COALESCE($3, api_key),
			status = COALESCE($4, status),
			updated_at = NOW()
		WHERE id = $5`,
		req.Name, req.URL, req.APIKey, req.Status, id)
	if err != nil {
		s.logger.Error("Failed to update site", "error", err)
		jsonError(w, "failed to update site", http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{"status": "updated"})
}

func (s *FederationService) deleteSite(w http.ResponseWriter, r *http.Request, id string) {
	if s.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, "DELETE FROM federated_sites WHERE id = $1", id)
	if err != nil {
		s.logger.Error("Failed to delete site", "error", err)
		jsonError(w, "failed to delete site", http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{"status": "deleted"})
}

type searchResult struct {
	SiteID    string `json:"site_id"`
	SiteName  string `json:"site_name"`
	CameraID  string `json:"camera_id"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	FilePath  string `json:"file_path"`
	FileSize  int64  `json:"file_size"`
}

type searchResponse struct {
	Results []searchResult `json:"results"`
}

func (s *FederationService) handleSearch(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	cameraID := r.URL.Query().Get("camera_id")
	startTime := r.URL.Query().Get("start_time")
	endTime := r.URL.Query().Get("end_time")

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	var sites []FederatedSite
	err := s.db.SelectContext(ctx, &sites,
		"SELECT id, name, url, api_key FROM federated_sites WHERE status = 'active'")
	if err != nil {
		s.logger.Error("Failed to query active sites", "error", err)
		jsonError(w, "failed to query federated sites", http.StatusInternalServerError)
		return
	}

	type siteResult struct {
		site FederatedSite
		resp *http.Response
		err  error
	}

	results := make(chan siteResult, len(sites))
	var wg sync.WaitGroup

	for _, site := range sites {
		wg.Add(1)
		go func(site FederatedSite) {
			defer wg.Done()

			u := fmt.Sprintf("%s/api/recordings?camera_id=%s&start_time=%s&end_time=%s",
				strings.TrimRight(site.URL, "/"), cameraID, startTime, endTime)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err != nil {
				results <- siteResult{site: site, err: err}
				return
			}
			if site.APIKey != "" {
				req.Header.Set("Authorization", "Bearer "+site.APIKey)
			}

			resp, err := s.httpClient.Do(req)
			results <- siteResult{site: site, resp: resp, err: err}
		}(site)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var merged []searchResult
	for res := range results {
		if res.err != nil {
			s.logger.Warn("Federated search request failed", "site", res.site.Name, "error", res.err)
			continue
		}
		if res.resp == nil {
			continue
		}

		var remoteResp struct {
			Recordings []struct {
				CameraID  string `json:"camera_id"`
				StartTime string `json:"start_time"`
				EndTime   string `json:"end_time"`
				FilePath  string `json:"file_path"`
				FileSize  int64  `json:"file_size"`
			} `json:"recordings"`
		}
		if err := json.NewDecoder(res.resp.Body).Decode(&remoteResp); err != nil {
			s.logger.Warn("Failed to decode federated search response", "site", res.site.Name, "error", err)
			res.resp.Body.Close()
			continue
		}
		res.resp.Body.Close()

		for _, rec := range remoteResp.Recordings {
			merged = append(merged, searchResult{
				SiteID:    res.site.ID,
				SiteName:  res.site.Name,
				CameraID:  rec.CameraID,
				StartTime: rec.StartTime,
				EndTime:   rec.EndTime,
				FilePath:  rec.FilePath,
				FileSize:  rec.FileSize,
			})
		}
	}

	if merged == nil {
		merged = []searchResult{}
	}

	jsonOK(w, map[string]interface{}{
		"results": merged,
		"total":   len(merged),
	})
}

func (s *FederationService) handlePlaybackProxy(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/federation/playback/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 1 || parts[0] == "" {
		jsonError(w, "site id required", http.StatusBadRequest)
		return
	}

	siteID := parts[0]
	recPath := ""
	if len(parts) == 2 {
		recPath = parts[1]
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var site FederatedSite
	err := s.db.GetContext(ctx, &site,
		"SELECT id, name, url, api_key FROM federated_sites WHERE id = $1 AND status = 'active'", siteID)
	if err != nil {
		jsonError(w, "remote site not found or inactive", http.StatusNotFound)
		return
	}

	u := fmt.Sprintf("%s/playback/%s", strings.TrimRight(site.URL, "/"), recPath)
	proxyReq, err := http.NewRequestWithContext(ctx, r.Method, u, r.Body)
	if err != nil {
		jsonError(w, "failed to create proxy request", http.StatusInternalServerError)
		return
	}
	proxyReq.Header = r.Header.Clone()
	if site.APIKey != "" {
		proxyReq.Header.Set("Authorization", "Bearer "+site.APIKey)
	}

	resp, err := s.httpClient.Do(proxyReq)
	if err != nil {
		s.logger.Error("Playback proxy request failed", "site", site.Name, "error", err)
		jsonError(w, "failed to proxy playback request", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (s *FederationService) measureLatency(baseURL string) int {
	start := time.Now()
	u := strings.TrimRight(baseURL, "/") + "/health"
	resp, err := s.httpClient.Get(u)
	if err != nil {
		return -1
	}
	resp.Body.Close()
	return int(time.Since(start).Milliseconds())
}

func main() {
	logger := common.NewLogger("federation")
	slog.SetDefault(logger)

	if err := common.InitTelemetry("federation"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultFederationConfig()

	common.StartMetricsServer(common.GetEnv("METRICS_ADDR", ":2112"))
	common.StartResourceMonitor(ctx)

	service, err := NewFederationService(config, logger)
	if err != nil {
		logger.Error("Failed to initialize federation service", "error", err)
		os.Exit(1)
	}

	go func() {
		if err := service.Start(); err != nil && err != http.ErrServerClosed {
			logger.Error("Federation service failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("Shutting down Federation Service...")

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := service.Shutdown(shutdownCtx); err != nil {
		logger.Error("Error during shutdown", "error", err)
	}
}
