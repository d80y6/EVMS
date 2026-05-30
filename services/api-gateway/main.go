package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dam-vms/dam/api/v1"
	"github.com/dam-vms/dam/pkg/common"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*clientLimit
	rate    float64
	burst   float64
	cleanup time.Duration
}

type clientLimit struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(rate, burst int, cleanup time.Duration) *rateLimiter {
	rl := &rateLimiter{
		clients: make(map[string]*clientLimit),
		rate:    float64(rate),
		burst:   float64(burst),
		cleanup: cleanup,
	}
	if cleanup > 0 {
		go rl.cleanupLoop()
	}
	return rl
}

func (rl *rateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, cl := range rl.clients {
			if now.Sub(cl.last) > rl.cleanup*2 {
				delete(rl.clients, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *rateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cl, ok := rl.clients[ip]
	if !ok {
		cl = &clientLimit{tokens: rl.burst, last: now}
		rl.clients[ip] = cl
	}

	elapsed := now.Sub(cl.last).Seconds()
	cl.tokens += elapsed * rl.rate
	if cl.tokens > rl.burst {
		cl.tokens = rl.burst
	}
	cl.last = now

	if cl.tokens >= 1 {
		cl.tokens--
		return true
	}
	return false
}

func (rl *rateLimiter) rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if !rl.Allow(host) {
			jsonError(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

type GatewayConfig struct {
	Port               string
	AuthServiceURL     string
	CameraServiceAddr  string
	PlaybackServiceURL string
	WebRTCServiceURL   string
	CameraControlURL   string
	DBURL              string
	MetricsAddr        string
}

func DefaultGatewayConfig() GatewayConfig {
	return GatewayConfig{
		Port:               common.GetEnv("GATEWAY_PORT", ":8090"),
		AuthServiceURL:     common.GetEnv("AUTH_SERVICE_URL", "http://auth-service:8081"),
		CameraServiceAddr:  common.GetEnv("CAMERA_SERVICE_ADDR", "camera-mgmt:50051"),
		PlaybackServiceURL: common.GetEnv("PLAYBACK_SERVICE_URL", "http://playback-service:8086"),
		WebRTCServiceURL:   common.GetEnv("WEBRTC_SERVICE_URL", "http://webrtc-service:8082"),
		CameraControlURL:   common.GetEnv("CAMERA_CONTROL_URL", "http://camera-control:8088"),
		DBURL:              common.GetEnv("DB_URL", ""),
		MetricsAddr:        common.GetEnv("METRICS_ADDR", ":2112"),
	}
}

type Gateway struct {
	config             GatewayConfig
	logger             *slog.Logger
	db                 *sqlx.DB
	cameraCC           *grpc.ClientConn
	cameraSvc          damv1.CameraServiceClient
	authProxy          *httputil.ReverseProxy
	playbackProxy      *httputil.ReverseProxy
	webrtcProxy        *httputil.ReverseProxy
	cameraControlProxy *httputil.ReverseProxy
	rateLimiter        *rateLimiter
}

func NewGateway(config GatewayConfig, logger *slog.Logger) (*Gateway, error) {
	var db *sqlx.DB
	if config.DBURL != "" {
		var err error
		db, err = sqlx.Connect("postgres", config.DBURL)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to DB: %w", err)
		}
	}

	cameraCC, err := grpc.NewClient(config.CameraServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to camera service: %w", err)
	}
	cameraSvc := damv1.NewCameraServiceClient(cameraCC)

	authURL, _ := url.Parse(config.AuthServiceURL)
	playbackURL, _ := url.Parse(config.PlaybackServiceURL)
	webrtcURL, _ := url.Parse(config.WebRTCServiceURL)
	cameraControlURL, _ := url.Parse(config.CameraControlURL)

	return &Gateway{
		config:             config,
		logger:             logger,
		db:                 db,
		cameraCC:           cameraCC,
		cameraSvc:          cameraSvc,
		authProxy:          httputil.NewSingleHostReverseProxy(authURL),
		playbackProxy:      httputil.NewSingleHostReverseProxy(playbackURL),
		webrtcProxy:        httputil.NewSingleHostReverseProxy(webrtcURL),
		cameraControlProxy: httputil.NewSingleHostReverseProxy(cameraControlURL),
		rateLimiter:        newRateLimiter(100, 200, 10*time.Minute),
	}, nil
}

func (g *Gateway) Close() error {
	if g.db != nil {
		g.db.Close()
	}
	if g.cameraCC != nil {
		g.cameraCC.Close()
	}
	return nil
}

func (g *Gateway) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			jsonError(w, "authorization required", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if _, err := common.ValidateJWT(token); err != nil {
			jsonError(w, "invalid token", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (g *Gateway) requireRole(minRole string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				jsonError(w, "authorization required", http.StatusUnauthorized)
				return
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := common.ValidateJWT(token)
			if err != nil {
				jsonError(w, "invalid token", http.StatusUnauthorized)
				return
			}

			roleLevels := map[string]int{
				"viewer":   1,
				"operator": 2,
				"admin":    3,
			}

			userLevel, ok := roleLevels[claims.Role]
			if !ok {
				userLevel = 0
			}
			requiredLevel, ok := roleLevels[minRole]
			if !ok {
				requiredLevel = 99
			}

			if userLevel < requiredLevel {
				jsonError(w, "insufficient permissions", http.StatusForbidden)
				return
			}

			next(w, r)
		}
	}
}

func (g *Gateway) handleLogin(w http.ResponseWriter, r *http.Request) {
	g.authProxy.ServeHTTP(w, r)
}

func (g *Gateway) handleCameras(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resp, err := g.cameraSvc.ListCameras(ctx, &damv1.ListCamerasRequest{})
	if err != nil {
		g.logger.Error("Failed to list cameras", "error", err)
		jsonError(w, "failed to list cameras", http.StatusInternalServerError)
		return
	}

	type cameraJSON struct {
		ID            string `json:"id"`
		SiteID        string `json:"site_id"`
		Name          string `json:"name"`
		Description   string `json:"description"`
		ConnectionURL string `json:"connection_url"`
		SubstreamURL  string `json:"substream_url"`
		Status        string `json:"status"`
	}

	cameras := make([]cameraJSON, len(resp.Cameras))
	for i, c := range resp.Cameras {
		cameras[i] = cameraJSON{
			ID:            c.Id,
			SiteID:        c.SiteId,
			Name:          c.Name,
			Description:   c.Description,
			ConnectionURL: c.ConnectionUrl,
			SubstreamURL:  c.SubstreamUrl,
			Status:        c.Status,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"cameras": cameras})
}

func (g *Gateway) handleRecordings(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	type recording struct {
		CameraID  string    `json:"camera_id" db:"camera_id"`
		StartTime time.Time `json:"start_time" db:"start_time"`
		EndTime   time.Time `json:"end_time" db:"end_time"`
		FilePath  string    `json:"file_path" db:"file_path"`
		FileSize  int64     `json:"file_size" db:"file_size"`
	}

	var recordings []recording
	err := g.db.SelectContext(ctx, &recordings,
		"SELECT camera_id, start_time, end_time, file_path, file_size FROM recordings ORDER BY start_time DESC LIMIT 100")
	if err != nil {
		g.logger.Error("Failed to query recordings", "error", err)
		jsonError(w, "failed to query recordings", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"recordings": recordings})
}

func (g *Gateway) handleEvents(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	type event struct {
		ID         string    `json:"id" db:"id"`
		CameraID   string    `json:"camera_id" db:"camera_id"`
		ObjectType string    `json:"object_type" db:"object_type"`
		Confidence float64   `json:"confidence" db:"confidence"`
		EventTime  time.Time `json:"event_time" db:"event_time"`
	}

	var events []event
	err := g.db.SelectContext(ctx, &events,
		"SELECT id, camera_id, object_type, confidence, event_time FROM ai_events ORDER BY event_time DESC LIMIT 100")
	if err != nil {
		g.logger.Error("Failed to query events", "error", err)
		jsonError(w, "failed to query events", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"events": events})
}

func (g *Gateway) handlePlayback(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
	g.playbackProxy.ServeHTTP(w, r)
}

func (g *Gateway) handleWebRTC(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
	g.webrtcProxy.ServeHTTP(w, r)
}

func (g *Gateway) handleCameraControl(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
	g.cameraControlProxy.ServeHTTP(w, r)
}

func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	path := r.URL.Path
	switch {
	case path == "/api/health":
		g.handleHealth(w, r)
	case path == "/api/login":
		g.rateLimiter.rateLimitMiddleware(g.handleLogin)(w, r)
	case path == "/api/cameras" && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameras))(w, r)
	case path == "/api/recordings" && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleRecordings))(w, r)
	case path == "/api/events" && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleEvents))(w, r)
	case strings.HasPrefix(path, "/api/playback/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handlePlayback))(w, r)
	case strings.HasPrefix(path, "/api/webrtc/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleWebRTC))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && (strings.Contains(path, "/ptz/") || strings.HasSuffix(path, "/ptz/presets")):
		g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(g.handleCameraControl))(w, r)
	default:
		jsonError(w, "not found", http.StatusNotFound)
	}
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultGatewayConfig()
	common.StartMetricsServer(config.MetricsAddr)

	gateway, err := NewGateway(config, logger)
	if err != nil {
		logger.Error("Failed to create gateway", "error", err)
		os.Exit(1)
	}
	defer gateway.Close()

	server := &http.Server{
		Addr:         config.Port,
		Handler:      gateway,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("API Gateway listening", "port", config.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Gateway server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("Shutting down API Gateway...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}
