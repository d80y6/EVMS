package main

import (
	"context"
	"crypto/tls"
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
	"golang.org/x/crypto/acme/autocert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
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
	ThumbnailsURL      string
	RecorderURL        string
	ExportURL          string
	AlertURL          string
	AuditURL          string
	POSURL            string
	DiscoveryURL      string
	OnvifEventsURL    string
	DBURL              string
	MetricsAddr        string
	TLSEnabled         bool
	TLSDomain          string
	TLSEmail           string
}

func DefaultGatewayConfig() GatewayConfig {
	return GatewayConfig{
		Port:               common.GetEnv("GATEWAY_PORT", ":8090"),
		AuthServiceURL:     common.GetEnv("AUTH_SERVICE_URL", "http://auth-service:8081"),
		CameraServiceAddr:  common.GetEnv("CAMERA_SERVICE_ADDR", "camera-mgmt:50051"),
		PlaybackServiceURL: common.GetEnv("PLAYBACK_SERVICE_URL", "http://playback-service:8086"),
		WebRTCServiceURL:   common.GetEnv("WEBRTC_SERVICE_URL", "http://webrtc-service:8082"),
		CameraControlURL:   common.GetEnv("CAMERA_CONTROL_URL", "http://camera-control:8088"),
		ThumbnailsURL:      common.GetEnv("THUMBNAILS_URL", "http://thumbnails:8089"),
		RecorderURL:        common.GetEnv("RECORDER_URL", "http://recorder-service:8087"),
		ExportURL:          common.GetEnv("EXPORT_URL", "http://export-service:8094"),
		AlertURL:          common.GetEnv("ALERT_URL", "http://event-proc:8093"),
		AuditURL:          common.GetEnv("AUDIT_URL", "http://audit-service:8093"),
		POSURL:            common.GetEnv("POS_URL", "http://pos-ingest:8096"),
		DiscoveryURL:      common.GetEnv("DISCOVERY_URL", "http://discovery:8091"),
		OnvifEventsURL:    common.GetEnv("ONVIF_EVENTS_URL", "http://onvif-events:8092"),
		DBURL:              common.GetEnv("DB_URL", ""),
		MetricsAddr:        common.GetEnv("METRICS_ADDR", ":2112"),
		TLSEnabled:         common.GetEnv("TLS_ENABLED", "false") == "true",
		TLSDomain:          common.GetEnv("TLS_DOMAIN", ""),
		TLSEmail:           common.GetEnv("TLS_EMAIL", ""),
	}
}

type upstreamHealth struct {
	Name string
	URL  string
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
	thumbnailsProxy    *httputil.ReverseProxy
	recorderProxy      *httputil.ReverseProxy
	exportProxy        *httputil.ReverseProxy
	alertProxy         *httputil.ReverseProxy
	auditProxy         *httputil.ReverseProxy
	posProxy           *httputil.ReverseProxy
	discoveryProxy     *httputil.ReverseProxy
	onvifEventsProxy   *httputil.ReverseProxy
	rateLimiter        *rateLimiter
	healthHandler      *common.HealthHandler
	upstreamHealth     []upstreamHealth
}

func NewGateway(config GatewayConfig, logger *slog.Logger) (*Gateway, error) {
	var db *sqlx.DB
	if config.DBURL != "" {
		cb := common.NewDBCircuitBreaker("api-gateway")
		var err error
		db, err = common.ConnectDBWithCircuitBreaker(context.Background(), "postgres", config.DBURL, cb)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to DB: %w", err)
		}
	}

	creds, err := common.GRPCClientTLSCredentials("camera-mgmt")
	if err != nil {
		return nil, fmt.Errorf("failed to configure gRPC credentials: %w", err)
	}
	cameraCC, err := grpc.NewClient(config.CameraServiceAddr,
		grpc.WithTransportCredentials(creds),
		grpc.WithUnaryInterceptor(tenantUnaryInterceptor),
		grpc.WithDefaultCallOptions(grpc.CallContentSubtype("json")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to camera service: %w", err)
	}
	cameraSvc := damv1.NewCameraServiceClient(cameraCC)

	authURL, _ := url.Parse(config.AuthServiceURL)
	authProxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = authURL.Scheme
			req.URL.Host = authURL.Host
			req.URL.Path = "/auth" + strings.TrimPrefix(req.URL.Path, "/api")
			if req.URL.RawQuery != "" {
				req.URL.RawQuery = req.URL.RawQuery
			}
		},
	}
	playbackURL, _ := url.Parse(config.PlaybackServiceURL)
	webrtcURL, _ := url.Parse(config.WebRTCServiceURL)
	cameraControlURL, _ := url.Parse(config.CameraControlURL)
	thumbnailsURL, _ := url.Parse(config.ThumbnailsURL)
	recorderURL, _ := url.Parse(config.RecorderURL)
	exportURL, _ := url.Parse(config.ExportURL)
	alertURL, _ := url.Parse(config.AlertURL)
	auditURL, _ := url.Parse(config.AuditURL)
	posURL, _ := url.Parse(config.POSURL)
	discoveryURL, _ := url.Parse(config.DiscoveryURL)
	onvifEventsURL, _ := url.Parse(config.OnvifEventsURL)

	h := common.NewHealthHandler()
	if db != nil {
		h.AddDBChecker(db.DB, "postgres")
	}

	return &Gateway{
		config:             config,
		logger:             logger,
		db:                 db,
		cameraCC:           cameraCC,
		cameraSvc:          cameraSvc,
		authProxy:          authProxy,
		playbackProxy:      httputil.NewSingleHostReverseProxy(playbackURL),
		webrtcProxy:        httputil.NewSingleHostReverseProxy(webrtcURL),
		cameraControlProxy: httputil.NewSingleHostReverseProxy(cameraControlURL),
		thumbnailsProxy:    httputil.NewSingleHostReverseProxy(thumbnailsURL),
		recorderProxy:      httputil.NewSingleHostReverseProxy(recorderURL),
		exportProxy:        httputil.NewSingleHostReverseProxy(exportURL),
		alertProxy:         httputil.NewSingleHostReverseProxy(alertURL),
		auditProxy:         httputil.NewSingleHostReverseProxy(auditURL),
		posProxy:           httputil.NewSingleHostReverseProxy(posURL),
		discoveryProxy:     httputil.NewSingleHostReverseProxy(discoveryURL),
		onvifEventsProxy:   httputil.NewSingleHostReverseProxy(onvifEventsURL),
		rateLimiter:        newRateLimiter(100, 200, 10*time.Minute),
		healthHandler:      h,
		upstreamHealth: []upstreamHealth{
			{"auth", config.AuthServiceURL + "/health"},
			{"playback", config.PlaybackServiceURL + "/health"},
			{"webrtc", config.WebRTCServiceURL + "/health"},
			{"camera-control", config.CameraControlURL + "/health"},
			{"thumbnails", config.ThumbnailsURL + "/health"},
			{"recorder", config.RecorderURL + "/health"},
			{"export", config.ExportURL + "/health"},
			{"event-proc", config.AlertURL + "/health"},
			{"audit", config.AuditURL + "/health"},
			{"camera-mgmt", "http://camera-mgmt:8083/health"},
			{"metadata", "http://metadata:8089/health"},
			{"notification", "http://notification:8090/health"},
			{"ingest", "http://ingest-service:8092/health"},
			{"pos-ingest", config.POSURL + "/health"},
			{"discovery", config.DiscoveryURL + "/health"},
			{"onvif-events", config.OnvifEventsURL + "/health"},
		},
	}, nil
}

func tenantUnaryInterceptor(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	if tenant := common.TenantFromContext(ctx); tenant != "" {
		md := metadata.Pairs("tenant_id", tenant)
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	return invoker(ctx, method, req, reply, cc, opts...)
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
		claims, err := common.ValidateJWT(token)
		if err != nil {
			jsonError(w, "invalid token", http.StatusUnauthorized)
			return
		}
		if claims.TenantID != "" {
			r.Header.Set("X-Tenant-ID", claims.TenantID)
		}
		r.Header.Set("X-Username", claims.Username)
		r.Header.Set("X-Role", claims.Role)
		ctx := r.Context()
		if claims.TenantID != "" {
			ctx = context.WithValue(ctx, common.TenantKey, claims.TenantID)
		}
		ctx = context.WithValue(ctx, common.UserKey, claims.Username)
		ctx = context.WithValue(ctx, common.RoleKey, claims.Role)
		r = r.WithContext(ctx)
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

			if claims.TenantID != "" {
				r.Header.Set("X-Tenant-ID", claims.TenantID)
			}
			r.Header.Set("X-Username", claims.Username)
			r.Header.Set("X-Role", claims.Role)
			ctx := r.Context()
			if claims.TenantID != "" {
				ctx = context.WithValue(ctx, common.TenantKey, claims.TenantID)
			}
			ctx = context.WithValue(ctx, common.UserKey, claims.Username)
			ctx = context.WithValue(ctx, common.RoleKey, claims.Role)
			r = r.WithContext(ctx)
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

	siteID := r.URL.Query().Get("site_id")
	resp, err := g.cameraSvc.ListCameras(ctx, &damv1.ListCamerasRequest{SiteId: siteID})
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
		PtzProtocol   string `json:"ptz_protocol"`
		RetentionDays int32  `json:"retention_days"`
		Config        string `json:"config"`
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
			PtzProtocol:   c.PtzProtocol,
			RetentionDays: c.RetentionDays,
			Config:        c.Config,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"cameras": cameras})
}

func (g *Gateway) handleListSites(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resp, err := g.cameraSvc.ListSites(ctx, &damv1.ListSitesRequest{})
	if err != nil {
		g.logger.Error("Failed to list sites", "error", err)
		jsonError(w, "failed to list sites", http.StatusInternalServerError)
		return
	}

	type siteJSON struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Location  string `json:"location"`
		CreatedAt string `json:"created_at"`
	}

	sites := make([]siteJSON, len(resp.Sites))
	for i, s := range resp.Sites {
		createdAt := ""
		if s.CreatedAt != nil {
			createdAt = s.CreatedAt.AsTime().Format(time.RFC3339)
		}
		sites[i] = siteJSON{
			ID:        s.Id,
			Name:      s.Name,
			Location:  s.Location,
			CreatedAt: createdAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"sites": sites})
}

func (g *Gateway) handleCreateSite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Location string `json:"location"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	site, err := g.cameraSvc.CreateSite(ctx, &damv1.CreateSiteRequest{
		Name:     req.Name,
		Location: req.Location,
	})
	if err != nil {
		g.logger.Error("Failed to create site", "error", err)
		jsonError(w, "failed to create site", http.StatusInternalServerError)
		return
	}

	createdAt := ""
	if site.CreatedAt != nil {
		createdAt = site.CreatedAt.AsTime().Format(time.RFC3339)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"site": map[string]interface{}{
			"id":         site.Id,
			"name":       site.Name,
			"location":   site.Location,
			"created_at": createdAt,
		},
	})
}

func (g *Gateway) handleSmartSearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CameraID      string  `json:"camera_id"`
		ObjectType    string  `json:"object_type"`
		MinConfidence float64 `json:"min_confidence"`
		StartTime     string  `json:"start_time"`
		EndTime       string  `json:"end_time"`
		Limit         int32   `json:"limit"`
		BoundingBox   string  `json:"bounding_box"`
	}

	if r.Method == http.MethodGet {
		req.CameraID = r.URL.Query().Get("camera_id")
		req.ObjectType = r.URL.Query().Get("object_type")
		req.StartTime = r.URL.Query().Get("start_time")
		req.EndTime = r.URL.Query().Get("end_time")
		req.BoundingBox = r.URL.Query().Get("bounding_box")
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	resp, err := g.cameraSvc.SmartSearch(ctx, &damv1.SmartSearchRequest{
		CameraId:      req.CameraID,
		ObjectType:    req.ObjectType,
		MinConfidence: req.MinConfidence,
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		Limit:         req.Limit,
		BoundingBox:   req.BoundingBox,
	})
	if err != nil {
		g.logger.Error("Failed to smart search", "error", err)
		jsonError(w, "failed to smart search", http.StatusInternalServerError)
		return
	}

	type resultJSON struct {
		ID          string  `json:"id"`
		CameraID    string  `json:"camera_id"`
		EventTime   string  `json:"event_time"`
		ObjectType  string  `json:"object_type"`
		Confidence  float64 `json:"confidence"`
		BoundingBox string  `json:"bounding_box"`
		TrackID     string  `json:"track_id"`
		Thumbnail   string  `json:"thumbnail"`
	}

	results := make([]resultJSON, len(resp.Results))
	for i, res := range resp.Results {
		results[i] = resultJSON{
			ID:          res.Id,
			CameraID:    res.CameraId,
			EventTime:   res.EventTime,
			ObjectType:  res.ObjectType,
			Confidence:  res.Confidence,
			BoundingBox: res.BoundingBox,
			TrackID:     res.TrackId,
			Thumbnail:   res.Thumbnail,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results": results,
		"total":   resp.Total,
	})
}

func (g *Gateway) handleRecordings(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	tenantID, _ := r.Context().Value(common.TenantKey).(string)

	type recording struct {
		CameraID  string    `json:"camera_id" db:"camera_id"`
		StartTime time.Time `json:"start_time" db:"start_time"`
		EndTime   time.Time `json:"end_time" db:"end_time"`
		FilePath  string    `json:"file_path" db:"file_path"`
		FileSize  int64     `json:"file_size" db:"file_size"`
	}

	var recordings []recording
	var err error
	if tenantID != "" {
		err = g.db.SelectContext(ctx, &recordings,
			`SELECT r.camera_id, r.start_time, r.end_time, r.file_path, r.file_size
			 FROM recordings r
			 JOIN cameras c ON r.camera_id = c.id
			 JOIN sites s ON c.site_id = s.id
			 WHERE s.tenant_id = $1
			 ORDER BY r.start_time DESC LIMIT 100`, tenantID)
	} else {
		err = g.db.SelectContext(ctx, &recordings,
			"SELECT camera_id, start_time, end_time, file_path, file_size FROM recordings ORDER BY start_time DESC LIMIT 100")
	}
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

	tenantID, _ := r.Context().Value(common.TenantKey).(string)

	type event struct {
		ID         string    `json:"id" db:"id"`
		CameraID   string    `json:"camera_id" db:"camera_id"`
		ObjectType string    `json:"object_type" db:"object_type"`
		Confidence float64   `json:"confidence" db:"confidence"`
		EventTime  time.Time `json:"event_time" db:"event_time"`
	}

	var events []event
	var err error
	if tenantID != "" {
		err = g.db.SelectContext(ctx, &events,
			`SELECT e.id, e.camera_id, e.object_type, e.confidence, e.event_time
			 FROM ai_events e
			 JOIN cameras c ON e.camera_id = c.id
			 JOIN sites s ON c.site_id = s.id
			 WHERE s.tenant_id = $1
			 ORDER BY e.event_time DESC LIMIT 100`, tenantID)
	} else {
		err = g.db.SelectContext(ctx, &events,
			"SELECT id, camera_id, object_type, confidence, event_time FROM ai_events ORDER BY event_time DESC LIMIT 100")
	}
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

func (g *Gateway) handleThumbnails(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
	g.thumbnailsProxy.ServeHTTP(w, r)
}

func extractParam(path, prefix string) string {
	return strings.TrimPrefix(path, prefix)
}

func (g *Gateway) handleStreamURL(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	cameraID := extractParam(r.URL.Path, "/api/stream/")
	streamType := r.URL.Query().Get("type")
	if streamType == "" {
		streamType = "main"
	}

	camera, err := g.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	url := camera.ConnectionUrl
	if streamType == "sub" && camera.SubstreamUrl != "" {
		url = camera.SubstreamUrl
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": url})
}

func (g *Gateway) handleGetCamera(w http.ResponseWriter, r *http.Request) {
	cameraID := extractParam(r.URL.Path, "/api/cameras/")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		g.logger.Error("Failed to get camera", "error", err)
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(camera)
}

func (g *Gateway) handleCreateCamera(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SiteID        string `json:"site_id"`
		Name          string `json:"name"`
		ConnectionURL string `json:"connection_url"`
		SubstreamURL  string `json:"substream_url"`
		PtzProtocol   string `json:"ptz_protocol"`
		RetentionDays int32  `json:"retention_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.CreateCamera(ctx, &damv1.CreateCameraRequest{
		SiteId:        req.SiteID,
		Name:          req.Name,
		ConnectionUrl: req.ConnectionURL,
		SubstreamUrl:  req.SubstreamURL,
		PtzProtocol:   req.PtzProtocol,
		RetentionDays: req.RetentionDays,
	})
	if err != nil {
		g.logger.Error("Failed to create camera", "error", err)
		jsonError(w, "failed to create camera", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(camera)
}

func (g *Gateway) handleUpdateCamera(w http.ResponseWriter, r *http.Request) {
	cameraID := extractParam(r.URL.Path, "/api/cameras/")

	var req struct {
		Name          string `json:"name"`
		ConnectionURL string `json:"connection_url"`
		SubstreamURL  string `json:"substream_url"`
		PtzProtocol   string `json:"ptz_protocol"`
		RetentionDays int32  `json:"retention_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.UpdateCamera(ctx, &damv1.UpdateCameraRequest{
		Id:            cameraID,
		Name:          req.Name,
		ConnectionUrl: req.ConnectionURL,
		SubstreamUrl:  req.SubstreamURL,
		PtzProtocol:   req.PtzProtocol,
		RetentionDays: req.RetentionDays,
	})
	if err != nil {
		g.logger.Error("Failed to update camera", "error", err)
		jsonError(w, "failed to update camera", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(camera)
}

func (g *Gateway) handleDeleteCamera(w http.ResponseWriter, r *http.Request) {
	cameraID := extractParam(r.URL.Path, "/api/cameras/")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, err := g.cameraSvc.DeleteCamera(ctx, &damv1.DeleteCameraRequest{Id: cameraID})
	if err != nil {
		g.logger.Error("Failed to delete camera", "error", err)
		jsonError(w, "failed to delete camera", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func (g *Gateway) handleUpdateCameraConfig(w http.ResponseWriter, r *http.Request) {
	cameraID := extractParam(r.URL.Path, "/api/cameras/")
	cameraID = strings.TrimSuffix(cameraID, "/config")

	var req struct {
		Config json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, err := g.cameraSvc.UpdateCamera(ctx, &damv1.UpdateCameraRequest{
		Id:     cameraID,
		Config: string(req.Config),
	})
	if err != nil {
		g.logger.Error("Failed to update camera config", "error", err)
		jsonError(w, "failed to update config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

func (g *Gateway) handleSystemHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	type upstreamResult struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}

	results := make([]upstreamResult, len(g.upstreamHealth))
	var wg sync.WaitGroup

	for i, us := range g.upstreamHealth {
		wg.Add(1)
		go func(i int, name, url string) {
			defer wg.Done()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				results[i] = upstreamResult{Name: name, Status: "down", Error: err.Error()}
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				results[i] = upstreamResult{Name: name, Status: "down", Error: err.Error()}
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				results[i] = upstreamResult{Name: name, Status: "ok"}
			} else {
				results[i] = upstreamResult{Name: name, Status: "degraded", Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
			}
		}(i, us.Name, us.URL)
	}
	wg.Wait()

	overall := "ok"
	for _, r := range results {
		if r.Status != "ok" {
			overall = "degraded"
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    overall,
		"timestamp": time.Now().UTC(),
		"services":  results,
	})
}

func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	if g.healthHandler != nil {
		g.healthHandler.Liveness(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (g *Gateway) handleReady(w http.ResponseWriter, r *http.Request) {
	if g.healthHandler != nil {
		g.healthHandler.Readiness(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if origin := r.Header.Get("Origin"); origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	path := r.URL.Path
	switch {
	case path == "/api/health/system":
		g.handleSystemHealth(w, r)
	case path == "/api/health":
		g.handleHealth(w, r)
	case path == "/api/ready":
		g.handleReady(w, r)
	case path == "/api/login":
		g.rateLimiter.rateLimitMiddleware(g.handleLogin)(w, r)
	case path == "/api/cameras" && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameras))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && !strings.Contains(path[len("/api/cameras/"):], "/") && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleGetCamera))(w, r)
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
	case strings.HasPrefix(path, "/api/cameras/") && strings.HasSuffix(path, "/io"):
		g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(g.handleCameraControl))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.HasSuffix(path, "/config") && r.Method == http.MethodPut:
		g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(g.handleUpdateCameraConfig))(w, r)
	case strings.HasPrefix(path, "/api/stream/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleStreamURL))(w, r)
	case strings.HasPrefix(path, "/api/thumbnails/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleThumbnails))(w, r)
	case strings.HasPrefix(path, "/api/admin/users"):
		g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(g.handleLogin))(w, r)
	case path == "/api/sites" && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleListSites))(w, r)
	case path == "/api/sites" && r.Method == http.MethodPost:
		g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(g.handleCreateSite))(w, r)
	case path == "/api/search":
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleSmartSearch))(w, r)
	case strings.HasPrefix(path, "/api/dewarp"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.recorderProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/storage/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.recorderProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/bookmarks"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			g.recorderProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/legal-holds"):
		g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.recorderProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/export"):
		g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.exportProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/alerts"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.alertProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/rules"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.alertProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/tours"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.cameraControlProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/analytics/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.alertProxy.ServeHTTP(w, r)
		}))(w, r)
	case path == "/api/audit/log" && r.Method == http.MethodPost:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			claims := common.ExtractClaims(r)
			if claims != nil {
				r.Header.Set("X-Actor", claims.Username)
			}
			g.auditProxy.ServeHTTP(w, r)
		}))(w, r)
	case path == "/api/audit/chain" && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			g.auditProxy.ServeHTTP(w, r)
		}))(w, r)
	case path == "/api/audit/verify" && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			g.auditProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/pos/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.posProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/discovery/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleDiscovery))(w, r)
	case strings.HasPrefix(path, "/api/onvif-events/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleOnvifEvents))(w, r)
	case path == "/api/cameras" && r.Method == http.MethodPost:
		g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(g.handleCreateCamera))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && !strings.Contains(path[len("/api/cameras/"):], "/") && r.Method == http.MethodPut:
		g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(g.handleUpdateCamera))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && !strings.Contains(path[len("/api/cameras/"):], "/") && r.Method == http.MethodDelete:
		g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(g.handleDeleteCamera))(w, r)
	default:
		jsonError(w, "not found", http.StatusNotFound)
	}
}

func (g *Gateway) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
	g.discoveryProxy.ServeHTTP(w, r)
}

func (g *Gateway) handleOnvifEvents(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
	g.onvifEventsProxy.ServeHTTP(w, r)
}

func serveTLS(ctx context.Context, config GatewayConfig, handler http.Handler, logger *slog.Logger) error {
	certManager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(config.TLSDomain),
		Email:      config.TLSEmail,
		Cache:      autocert.DirCache(".cache/autocert"),
	}

	redirectServer := &http.Server{
		Addr:         ":80",
		Handler:      certManager.HTTPHandler(nil),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("Starting HTTP redirect server", "addr", ":80")
		if err := redirectServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP redirect server error", "error", err)
		}
	}()

	tlsServer := &http.Server{
		Addr:         ":443",
		Handler:      common.RecoveryMiddleware(handler),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
		TLSConfig: &tls.Config{
			GetCertificate: certManager.GetCertificate,
			MinVersion:     tls.VersionTLS12,
		},
	}

	go func() {
		logger.Info("API Gateway listening with TLS", "domain", config.TLSDomain, "addr", ":443")
		if err := tlsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			logger.Error("TLS server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("Shutting down API Gateway...")

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := tlsServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("TLS server shutdown error", "error", err)
	}

	redirectCtx, redirectCancel := context.WithTimeout(ctx, 5*time.Second)
	defer redirectCancel()
	redirectServer.Shutdown(redirectCtx)

	return nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := common.InitTelemetry("api-gateway"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	config := DefaultGatewayConfig()
	common.StartMetricsServer(config.MetricsAddr)
	common.StartResourceMonitor(ctx)

	gateway, err := NewGateway(config, logger)
	if err != nil {
		logger.Error("Failed to create gateway", "error", err)
		os.Exit(1)
	}
	defer gateway.Close()

	if config.TLSEnabled {
		if err := serveTLS(ctx, config, gateway, logger); err != nil {
			logger.Error("TLS server failed", "error", err)
			os.Exit(1)
		}
		return
	}

	server := &http.Server{
		Addr:         config.Port,
		Handler:      common.RecoveryMiddleware(gateway),
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

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}
