package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dam-vms/dam/api/v1"
	"github.com/dam-vms/dam/pkg/common"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nats-io/nats.go"
	"github.com/dam-vms/dam/pkg/onvif"
	"github.com/sony/gobreaker"
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
	mu         sync.Mutex
	clients    map[string]*clientLimit
	userLimits map[string]*clientLimit
	rate       float64
	burst      float64
	userRate   float64
	userBurst  float64
	tenantRate float64
	tenantBurst float64
	cleanup    time.Duration
}

type clientLimit struct {
	tokens float64
	last   time.Time
}

// TODO: Implement Redis-backed rate limiter
// func newRedisRateLimiter(addr string) *rateLimiter {
// 	client := redis.NewClient(&redis.Options{Addr: addr})
// 	_ = client
// 	return newRateLimiter(100, 200, 10*time.Minute)
// }

func newRateLimiter(rate, burst int, cleanup time.Duration) *rateLimiter {
	rl := &rateLimiter{
		clients:     make(map[string]*clientLimit),
		userLimits:  make(map[string]*clientLimit),
		rate:        float64(rate),
		burst:       float64(burst),
		userRate:    300,
		userBurst:   100,
		tenantRate:  5000,
		tenantBurst: 1000,
		cleanup:     cleanup,
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
		for key, cl := range rl.userLimits {
			if now.Sub(cl.last) > rl.cleanup*2 {
				delete(rl.userLimits, key)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *rateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cl, ok := rl.clients[key]
	if !ok {
		cl = &clientLimit{tokens: rl.burst, last: now}
		rl.clients[key] = cl
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

func (rl *rateLimiter) AllowUser(userKey string) (bool, float64, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cl, ok := rl.userLimits[userKey]
	if !ok {
		cl = &clientLimit{tokens: rl.userBurst, last: now}
		rl.userLimits[userKey] = cl
	}

	elapsed := now.Sub(cl.last).Seconds()
	cl.tokens += elapsed * rl.userRate
	if cl.tokens > rl.userBurst {
		cl.tokens = rl.userBurst
	}
	cl.last = now

	remaining := cl.tokens
	if cl.tokens >= 1 {
		cl.tokens--
		remaining = cl.tokens
	}

	resetAt := cl.last.Add(time.Duration((rl.userBurst - cl.tokens) / rl.userRate * float64(time.Second)))
	return cl.tokens >= 0, remaining, time.Until(resetAt)
}

func (rl *rateLimiter) AllowTenant(tenantKey string) (bool, float64, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cl, ok := rl.userLimits["tenant:"+tenantKey]
	if !ok {
		cl = &clientLimit{tokens: rl.tenantBurst, last: now}
		rl.userLimits["tenant:"+tenantKey] = cl
	}

	elapsed := now.Sub(cl.last).Seconds()
	cl.tokens += elapsed * rl.tenantRate
	if cl.tokens > rl.tenantBurst {
		cl.tokens = rl.tenantBurst
	}
	cl.last = now

	remaining := cl.tokens
	if cl.tokens >= 1 {
		cl.tokens--
		remaining = cl.tokens
	}

	resetAt := cl.last.Add(time.Duration((rl.tenantBurst - cl.tokens) / rl.tenantRate * float64(time.Second)))
	return cl.tokens >= 0, remaining, time.Until(resetAt)
}

func extractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

func (rl *rateLimiter) rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := extractClientIP(r)
		if !rl.Allow(host) {
			jsonError(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

func generateCSRFToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (g *Gateway) csrfMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next(w, r)
			return
		}

		headerToken := r.Header.Get("X-CSRF-Token")
		cookie, err := r.Cookie("csrf_token")
		if err != nil || cookie.Value == "" {
			jsonError(w, "CSRF token required", http.StatusForbidden)
			return
		}

		if subtle.ConstantTimeCompare([]byte(headerToken), []byte(cookie.Value)) != 1 {
			jsonError(w, "invalid CSRF token", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}

func (g *Gateway) handleCSRFToken(w http.ResponseWriter, r *http.Request) {
	token := generateCSRFToken()
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   86400,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"csrf_token": token})
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
	FederationURL     string
	AlertURL          string
	AuditURL          string
	POSURL            string
	DiscoveryURL      string
	OnvifEventsURL    string
	NotificationURL   string
	ReportingURL      string
	ModelRegistryURL string
	DBURL              string
	RedisURL           string
	NATSURL            string
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
		FederationURL:      common.GetEnv("FEDERATION_URL", "http://federation:8099"),
		AlertURL:          common.GetEnv("ALERT_URL", "http://event-proc:8093"),
		AuditURL:          common.GetEnv("AUDIT_URL", "http://audit-service:8093"),
		POSURL:            common.GetEnv("POS_URL", "http://pos-ingest:8096"),
		DiscoveryURL:      common.GetEnv("DISCOVERY_URL", "http://discovery:8091"),
		OnvifEventsURL:    common.GetEnv("ONVIF_EVENTS_URL", "http://onvif-events:8092"),
		NotificationURL:   common.GetEnv("NOTIFICATION_URL", "http://notification:8090"),
		ReportingURL:      common.GetEnv("REPORTING_URL", "http://reporting-service:8098"),
		ModelRegistryURL: common.GetEnv("MODEL_REGISTRY_URL", "http://model-registry:8098"),
		DBURL:              common.GetEnv("DB_URL", ""),
		RedisURL:           common.GetEnv("REDIS_URL", ""),
		NATSURL:            common.GetEnv("NATS_URL", "nats://nats:4222"),
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

type cameraResponse struct {
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
	federationProxy    *httputil.ReverseProxy
	onvifEventsProxy   *httputil.ReverseProxy
	notificationProxy  *httputil.ReverseProxy
	reportingProxy     *httputil.ReverseProxy
	modelRegistryProxy *httputil.ReverseProxy
	rateLimiter        *rateLimiter
	ipAllowlist        *common.IPAllowlist
	licenseValidator   *common.LicenseValidator
	licenseClaims      *common.LicenseClaims
	healthHandler      *common.HealthHandler
	upstreamHealth     []upstreamHealth
	nc                 *nats.Conn
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

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	defaultTimeout := 10 * time.Second
	defaultRetries := 3

	cbAuth := common.NewHTTPCircuitBreaker("auth")
	cbPlayback := common.NewHTTPCircuitBreaker("playback")
	cbWebRTC := common.NewHTTPCircuitBreaker("webrtc")
	cbCameraControl := common.NewHTTPCircuitBreaker("camera-control")
	cbThumbnails := common.NewHTTPCircuitBreaker("thumbnails")
	cbRecorder := common.NewHTTPCircuitBreaker("recorder")
	cbExport := common.NewHTTPCircuitBreaker("export")
	cbAlert := common.NewHTTPCircuitBreaker("alert")
	cbAudit := common.NewHTTPCircuitBreaker("audit")
	cbPOS := common.NewHTTPCircuitBreaker("pos")
	cbDiscovery := common.NewHTTPCircuitBreaker("discovery")
	cbFederation := common.NewHTTPCircuitBreaker("federation")
	cbOnvifEvents := common.NewHTTPCircuitBreaker("onvif-events")
	cbNotification := common.NewHTTPCircuitBreaker("notification")
	cbReporting := common.NewHTTPCircuitBreaker("reporting")
	cbModelRegistry := common.NewHTTPCircuitBreaker("model-registry")

	makeCBTransport := func(cb *gobreaker.CircuitBreaker) http.RoundTripper {
		return common.NewCircuitBreakerTransport(cb, defaultTimeout, defaultRetries, transport)
	}

	authURL, _ := url.Parse(config.AuthServiceURL)
	authProxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = authURL.Scheme
			req.URL.Host = authURL.Host
			req.URL.Path = "/auth" + strings.TrimPrefix(req.URL.Path, "/api")
		},
		Transport: makeCBTransport(cbAuth),
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
	federationURL, _ := url.Parse(config.FederationURL)
	onvifEventsURL, _ := url.Parse(config.OnvifEventsURL)
	notificationURL, _ := url.Parse(config.NotificationURL)
	reportingURL, _ := url.Parse(config.ReportingURL)
	modelRegistryURL, _ := url.Parse(config.ModelRegistryURL)

	makeProxy := func(target *url.URL, cb *gobreaker.CircuitBreaker) *httputil.ReverseProxy {
		p := httputil.NewSingleHostReverseProxy(target)
		p.Transport = makeCBTransport(cb)
		return p
	}

	var nc *nats.Conn
	if config.NATSURL != "" {
		natsOpts := common.NATSAuthOptions()
		nc, err = nats.Connect(config.NATSURL, natsOpts...)
		if err != nil {
			logger.Warn("Failed to connect to NATS for plugin events", "error", err)
		}
	}

	ipAllowlist := common.NewIPAllowlist()
	ipAllowlist.SetEnabled(false)
	if db != nil {
		var cidrs []struct{ CIDR string }
		if err := db.Select(&cidrs, "SELECT cidr FROM ip_allowlist"); err == nil {
			for _, row := range cidrs {
				ipAllowlist.AddCIDR(row.CIDR)
			}
			if len(cidrs) > 0 {
				ipAllowlist.SetEnabled(true)
			}
		}
	}

	licenseKey := os.Getenv("LICENSE_KEY")
	var licenseValidator *common.LicenseValidator
	var licenseClaims *common.LicenseClaims
	if licenseKey != "" {
		pubKey := os.Getenv("LICENSE_PUBLIC_KEY")
		if pubKey != "" {
			pubBytes, _ := hex.DecodeString(pubKey)
			if len(pubBytes) == 32 {
				licenseValidator = common.NewLicenseValidator(pubBytes)
				claims, err := licenseValidator.ValidateLicense(licenseKey)
				if err == nil {
					licenseClaims = claims
				}
			}
		}
	}

	h := common.NewHealthHandler()
	if db != nil {
		h.AddDBChecker(db.DB, "postgres")
	}
	if nc != nil {
		h.AddNATSChecker(nc, "nats")
	}

	return &Gateway{
		config:             config,
		logger:             logger,
		db:                 db,
		cameraCC:           cameraCC,
		cameraSvc:          cameraSvc,
		authProxy:          authProxy,
		playbackProxy:      makeProxy(playbackURL, cbPlayback),
		webrtcProxy:        makeProxy(webrtcURL, cbWebRTC),
		cameraControlProxy: makeProxy(cameraControlURL, cbCameraControl),
		thumbnailsProxy:    makeProxy(thumbnailsURL, cbThumbnails),
		recorderProxy:      makeProxy(recorderURL, cbRecorder),
		exportProxy:        makeProxy(exportURL, cbExport),
		alertProxy:         makeProxy(alertURL, cbAlert),
		auditProxy:         makeProxy(auditURL, cbAudit),
		posProxy:           makeProxy(posURL, cbPOS),
		discoveryProxy:      makeProxy(discoveryURL, cbDiscovery),
		federationProxy:     makeProxy(federationURL, cbFederation),
		onvifEventsProxy:   makeProxy(onvifEventsURL, cbOnvifEvents),
		notificationProxy: makeProxy(notificationURL, cbNotification),
		reportingProxy:    makeProxy(reportingURL, cbReporting),
		modelRegistryProxy: makeProxy(modelRegistryURL, cbModelRegistry),
		nc:                 nc,
		// TODO: if config.RedisURL != "" { use newRedisRateLimiter(config.RedisURL) } else { use in-memory }
		rateLimiter:        newRateLimiter(100, 200, 10*time.Minute),
		ipAllowlist:        ipAllowlist,
		licenseValidator:   licenseValidator,
		licenseClaims:      licenseClaims,
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
			{"ingest", "http://ingest-service:8092/health"},
			{"pos-ingest", config.POSURL + "/health"},
			{"discovery", config.DiscoveryURL + "/health"},
			{"notification", config.NotificationURL + "/health"},
			{"onvif-events", config.OnvifEventsURL + "/health"},
			{"federation", config.FederationURL + "/health"},
			{"reporting", config.ReportingURL + "/health"},
			{"model-registry", config.ModelRegistryURL + "/health"},
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
	if g.nc != nil {
		g.nc.Close()
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
		r.Header.Set("Authorization", "Bearer "+token)
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
			r.Header.Set("Authorization", "Bearer "+token)
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
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
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

func (g *Gateway) handleDeleteSite(w http.ResponseWriter, r *http.Request) {
	siteID := strings.TrimPrefix(r.URL.Path, "/api/sites/")
	siteID = strings.TrimSuffix(siteID, "/")
	if siteID == "" {
		jsonError(w, "site ID required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var resp damv1.DeleteSiteResponse
	err := g.cameraCC.Invoke(ctx, "/dam_v1.CameraService/DeleteSite", &damv1.DeleteSiteRequest{Id: siteID}, &resp)
	if err != nil {
		g.logger.Error("Failed to delete site", "error", err, "id", siteID)
		jsonError(w, "failed to delete site", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": resp.Success,
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
		if mc := r.URL.Query().Get("min_confidence"); mc != "" {
			if v, err := strconv.ParseFloat(mc, 64); err == nil {
				req.MinConfidence = v
			}
		}
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.ParseInt(l, 10, 32); err == nil {
				req.Limit = int32(v)
			}
		}
	} else {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
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

	if tenantID == "" {
		jsonError(w, "tenant isolation required", http.StatusForbidden)
		return
	}

	var recordings []recording
	err := g.db.SelectContext(ctx, &recordings,
		`SELECT r.camera_id, r.start_time, r.end_time, r.file_path, r.file_size
		 FROM recordings r
		 JOIN cameras c ON r.camera_id = c.id
		 JOIN sites s ON c.site_id = s.id
		 WHERE s.tenant_id = $1
		 ORDER BY r.start_time DESC LIMIT 100`, tenantID)
	if err != nil {
		g.logger.Error("Failed to query recordings", "error", err)
		jsonError(w, "failed to query recordings", http.StatusInternalServerError)
		return
	}
	if recordings == nil {
		recordings = []recording{}
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

	if tenantID == "" {
		jsonError(w, "tenant isolation required", http.StatusForbidden)
		return
	}

	var events []event
	err := g.db.SelectContext(ctx, &events,
		`SELECT e.id, e.camera_id, e.object_type, e.confidence, e.event_time
		 FROM ai_events e
		 JOIN cameras c ON e.camera_id = c.id
		 JOIN sites s ON c.site_id = s.id
		 WHERE s.tenant_id = $1
		 ORDER BY e.event_time DESC LIMIT 100`, tenantID)
	if err != nil {
		g.logger.Error("Failed to query events", "error", err)
		jsonError(w, "failed to query events", http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = []event{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"events": events})
}

func (g *Gateway) handlePlayback(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := r.Context().Value(common.TenantKey).(string)
	if tenantID == "" {
		jsonError(w, "tenant isolation required", http.StatusForbidden)
		return
	}

	if g.db != nil {
		pathParts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/api/playback/"), "/", 2)
		if len(pathParts) > 0 && pathParts[0] != "" {
			var count int
			err := g.db.GetContext(r.Context(), &count,
				"SELECT COUNT(*) FROM cameras c JOIN sites s ON c.site_id = s.id WHERE c.id = $1 AND s.tenant_id = $2",
				pathParts[0], tenantID)
			if err != nil || count == 0 {
				jsonError(w, "forbidden", http.StatusForbidden)
				return
			}
		}
	}
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
	json.NewEncoder(w).Encode(cameraResponse{
		ID:            camera.Id,
		SiteID:        camera.SiteId,
		Name:          camera.Name,
		Description:   camera.Description,
		ConnectionURL: camera.ConnectionUrl,
		SubstreamURL:  camera.SubstreamUrl,
		Status:        camera.Status,
		PtzProtocol:   camera.PtzProtocol,
		RetentionDays: camera.RetentionDays,
		Config:        camera.Config,
	})
}

func (g *Gateway) handleCreateCamera(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		SiteID         string `json:"site_id"`
		Name           string `json:"name"`
		ConnectionURL  string `json:"connection_url"`
		SubstreamURL   string `json:"substream_url"`
		PtzProtocol    string `json:"ptz_protocol"`
		RetentionDays  int32  `json:"retention_days"`
		OnvifUsername  string `json:"onvif_username"`
		OnvifPassword  string `json:"onvif_password"`
		Description    string `json:"description"`
		PrerecordSeconds int32 `json:"prerecord_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Name) == "" || len(req.Name) > 255 || strings.ContainsAny(req.Name, "<>") {
		jsonError(w, "invalid camera name: must be non-empty, max 255 characters, no HTML tags", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.CreateCamera(ctx, &damv1.CreateCameraRequest{
		SiteId:           req.SiteID,
		Name:             req.Name,
		ConnectionUrl:    req.ConnectionURL,
		SubstreamUrl:     req.SubstreamURL,
		PtzProtocol:      req.PtzProtocol,
		RetentionDays:    req.RetentionDays,
		OnvifUsername:    req.OnvifUsername,
		OnvifPassword:    req.OnvifPassword,
		Description:      req.Description,
		PrerecordSeconds: req.PrerecordSeconds,
	})
	if err != nil {
		g.logger.Error("Failed to create camera", "error", err)
		jsonError(w, "failed to create camera", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(cameraResponse{
		ID:            camera.Id,
		SiteID:        camera.SiteId,
		Name:          camera.Name,
		Description:   camera.Description,
		ConnectionURL: camera.ConnectionUrl,
		SubstreamURL:  camera.SubstreamUrl,
		Status:        camera.Status,
		PtzProtocol:   camera.PtzProtocol,
		RetentionDays: camera.RetentionDays,
		Config:        camera.Config,
	})
}

func (g *Gateway) handleUpdateCamera(w http.ResponseWriter, r *http.Request) {
	cameraID := extractParam(r.URL.Path, "/api/cameras/")

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		Name           string `json:"name"`
		SiteID         string `json:"site_id"`
		Description    string `json:"description"`
		ConnectionURL  string `json:"connection_url"`
		SubstreamURL   string `json:"substream_url"`
		PtzProtocol    string `json:"ptz_protocol"`
		RetentionDays  int32  `json:"retention_days"`
		OnvifUsername  string `json:"onvif_username"`
		OnvifPassword  string `json:"onvif_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Name) == "" || len(req.Name) > 255 || strings.ContainsAny(req.Name, "<>") {
		jsonError(w, "invalid camera name: must be non-empty, max 255 characters, no HTML tags", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.UpdateCamera(ctx, &damv1.UpdateCameraRequest{
		Id:            cameraID,
		SiteId:        req.SiteID,
		Name:          req.Name,
		Description:   req.Description,
		ConnectionUrl: req.ConnectionURL,
		SubstreamUrl:  req.SubstreamURL,
		PtzProtocol:   req.PtzProtocol,
		RetentionDays: req.RetentionDays,
		OnvifUsername: req.OnvifUsername,
		OnvifPassword: req.OnvifPassword,
	})
	if err != nil {
		g.logger.Error("Failed to update camera", "error", err)
		jsonError(w, "failed to update camera", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cameraResponse{
		ID:            camera.Id,
		SiteID:        camera.SiteId,
		Name:          camera.Name,
		Description:   camera.Description,
		ConnectionURL: camera.ConnectionUrl,
		SubstreamURL:  camera.SubstreamUrl,
		Status:        camera.Status,
		PtzProtocol:   camera.PtzProtocol,
		RetentionDays: camera.RetentionDays,
		Config:        camera.Config,
	})
}

func (g *Gateway) handleCameraCredentials(w http.ResponseWriter, r *http.Request) {
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
	json.NewEncoder(w).Encode(map[string]string{
		"onvif_username": camera.OnvifUsername,
		"onvif_password": camera.OnvifPassword,
	})
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

func (g *Gateway) handleCameraDetails(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/cameras/")
	cameraID := strings.TrimSuffix(path, "/details")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	ipAddress := camera.ConnectionUrl
	if strings.HasPrefix(ipAddress, "rtsp://") {
		ipAddress = strings.TrimPrefix(ipAddress, "rtsp://")
		if idx := strings.Index(ipAddress, "@"); idx != -1 {
			ipAddress = ipAddress[idx+1:]
		}
		if idx := strings.Index(ipAddress, ":"); idx != -1 {
			ipAddress = ipAddress[:idx]
		}
	}

	siteName := ""
	var manufacturer, model, firmware, serialNumber, hwID string
	if g.db != nil {
		g.db.GetContext(ctx, &siteName, "SELECT name FROM sites WHERE id=$1", camera.SiteId)
		g.db.GetContext(ctx, &manufacturer,
			"SELECT COALESCE(onvif_data->>'manufacturer','') FROM cameras WHERE id=$1", cameraID)
		g.db.GetContext(ctx, &model,
			"SELECT COALESCE(onvif_data->>'model','') FROM cameras WHERE id=$1", cameraID)
		g.db.GetContext(ctx, &firmware,
			"SELECT COALESCE(onvif_data->>'firmware','') FROM cameras WHERE id=$1", cameraID)
		g.db.GetContext(ctx, &serialNumber,
			"SELECT COALESCE(onvif_data->>'serial_number','') FROM cameras WHERE id=$1", cameraID)
		g.db.GetContext(ctx, &hwID,
			"SELECT COALESCE(onvif_data->>'hardware_id','') FROM cameras WHERE id=$1", cameraID)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":             camera.Id,
		"name":           camera.Name,
		"description":    camera.Description,
		"site_id":        camera.SiteId,
		"site_name":      siteName,
		"ip_address":     ipAddress,
		"status":         camera.Status,
		"connection_url": camera.ConnectionUrl,
		"ptz_protocol":   camera.PtzProtocol,
		"retention_days": camera.RetentionDays,
		"created_at":     camera.CreatedAt,
		"manufacturer":   manufacturer,
		"model":          model,
		"firmware":       firmware,
		"serial_number":  serialNumber,
		"hardware_id":    hwID,
	})
}

func (g *Gateway) handleCameraStreams(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/cameras/")
	cameraID := strings.TrimSuffix(path, "/streams")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	profiles := []map[string]interface{}{
		{
			"token":      "main",
			"name":       "Main Stream",
			"url":        camera.ConnectionUrl,
			"resolution": "1920x1080",
			"fps":        30,
			"codec":      "H.264",
			"encoding":   "H.264",
			"width":      1920,
			"height":     1080,
			"bitrate":    4096,
		},
	}
	if camera.SubstreamUrl != "" {
		profiles = append(profiles, map[string]interface{}{
			"token":      "sub",
			"name":       "Sub Stream",
			"url":        camera.SubstreamUrl,
			"resolution": "704x480",
			"fps":        15,
			"codec":      "H.264",
			"encoding":   "H.264",
			"width":      704,
			"height":     480,
			"bitrate":    1024,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"main_stream": camera.ConnectionUrl,
		"sub_stream":  camera.SubstreamUrl,
		"profiles":    profiles,
	})
}

func (g *Gateway) handleCameraPTZ(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/cameras/")
	cameraID := strings.TrimSuffix(path, "/ptz")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	protocol := camera.PtzProtocol
	if protocol == "" {
		protocol = "NONE"
	}
	hasPTZ := protocol != "NONE"

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"protocol":        protocol,
		"supported":       hasPTZ,
		"absolute_move":   hasPTZ,
		"relative_move":   hasPTZ,
		"continuous_move": hasPTZ,
		"presets":         []interface{}{},
	})
}

func (g *Gateway) handleCameraNetwork(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/cameras/")
	cameraID := strings.TrimSuffix(path, "/network")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	ipAddress := ""
	rtspPort := 554

	connURL := camera.ConnectionUrl
	if strings.HasPrefix(connURL, "rtsp://") {
		host := strings.TrimPrefix(connURL, "rtsp://")
		if idx := strings.Index(host, "@"); idx != -1 {
			host = host[idx+1:]
		}
		if idx := strings.Index(host, ":"); idx != -1 {
			ipAddress = host[:idx]
			pStr := host[idx+1:]
			if p, err := strconv.Atoi(pStr); err == nil {
				rtspPort = p
			}
		} else {
			ipAddress = host
		}
	}

	interfaces := []map[string]interface{}{}
	if ipAddress != "" {
		interfaces = append(interfaces, map[string]interface{}{
			"name": "eth0",
			"ipv4": ipAddress,
			"mac":  "",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	onvifPort := 80
	onvifEnabled := true
	if camera.Config != "" {
		var cfg struct {
			OnvifPort int  `json:"onvif_port"`
			IsOnvif   bool `json:"is_onvif"`
		}
		if err := json.Unmarshal([]byte(camera.Config), &cfg); err == nil {
			if cfg.OnvifPort > 0 {
				onvifPort = cfg.OnvifPort
			}
			onvifEnabled = cfg.IsOnvif
		}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hostname":       "",
		"dns":            []string{},
		"ntp":            []string{},
		"interfaces":     interfaces,
		"ip_address":     ipAddress,
		"rtsp_port":      rtspPort,
		"onvif_port":     onvifPort,
		"onvif_enabled":  onvifEnabled,
		"http_port":      80,
		"dhcp":           true,
	})
}

func (g *Gateway) handleCameraDiagnostics(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/cameras/")
	cameraID := strings.TrimSuffix(path, "/diagnostics")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	reachable := camera.Status == "online"

	onvifEnabled := true
	if camera.Config != "" {
		var cfg struct {
			IsOnvif bool `json:"is_onvif"`
		}
		if err := json.Unmarshal([]byte(camera.Config), &cfg); err == nil {
			onvifEnabled = cfg.IsOnvif
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"reachable":      reachable,
		"onvif":          onvifEnabled && reachable,
		"onvif_enabled":  onvifEnabled,
		"rtsp":           reachable,
		"latency_ms":     0,
		"last_error":     "",
		"status":         camera.Status,
		"uptime_pct":     99.5,
		"response_time_ms": 45,
	})
}

func (g *Gateway) handleCameraRecording(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/cameras/")
	cameraID := strings.TrimSuffix(path, "/recording")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	totalRecordings := int64(0)
	storageUsed := int64(0)
	var oldestRecording, latestRecording string
	if g.db != nil {
		var stats struct {
			Count  int64    `db:"count"`
			Size   int64    `db:"size"`
			Oldest *string  `db:"oldest"`
			Latest *string  `db:"latest"`
		}
		err := g.db.GetContext(ctx, &stats,
			`SELECT COUNT(*) as count,
			        COALESCE(SUM(file_size), 0) as size,
			        MIN(start_time)::text as oldest,
			        MAX(end_time)::text as latest
			 FROM recordings WHERE camera_id=$1`, cameraID)
		if err == nil {
			totalRecordings = stats.Count
			storageUsed = stats.Size
			if stats.Oldest != nil {
				oldestRecording = *stats.Oldest
			}
			if stats.Latest != nil {
				latestRecording = *stats.Latest
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"retention_days":          camera.RetentionDays,
		"recordings_count":        totalRecordings,
		"oldest_recording":        oldestRecording,
		"latest_recording":        latestRecording,
		"prerecord_seconds":       camera.PrerecordSeconds,
		"recording_enabled":       true,
		"storage_used_bytes":      storageUsed,
		"storage_available_bytes": int64(0),
	})
}

func (g *Gateway) handleCameraOnvif(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/cameras/")
	cameraID := strings.TrimSuffix(path, "/onvif")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	deviceURL := camera.ConnectionUrl
	if strings.HasPrefix(deviceURL, "rtsp://") {
		onvifPort := 80
		if camera.Config != "" {
			var cfg struct {
				OnvifPort int `json:"onvif_port"`
			}
			if err := json.Unmarshal([]byte(camera.Config), &cfg); err == nil && cfg.OnvifPort > 0 {
				onvifPort = cfg.OnvifPort
			}
		}
		deviceURL = onvif.BuildDeviceURL(camera.ConnectionUrl, onvifPort)
	}

	capabilities := map[string]interface{}{}
	eventsSupported := true
	analyticsSupported := true
	if camera.OnvifData != "" {
		var onvifData map[string]interface{}
		if json.Unmarshal([]byte(camera.OnvifData), &onvifData) == nil {
			if caps, ok := onvifData["capabilities"].(map[string]interface{}); ok {
				capabilities = caps
			}
			if caps, ok := onvifData["capabilities"].(map[string]interface{}); ok {
				if v, ok := caps["events"].(bool); ok {
					eventsSupported = v
				}
				if v, ok := caps["analytics"].(bool); ok {
					analyticsSupported = v
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"capabilities":        capabilities,
		"events_supported":    eventsSupported,
		"analytics_supported": analyticsSupported,
		"device_uri":          deviceURL,
		"analytics":           analyticsSupported,
		"events":              eventsSupported,
		"ptz":                 camera.PtzProtocol != "NONE" && camera.PtzProtocol != "",
		"imaging":             true,
	})
}

func (g *Gateway) handleDiscoveryScan(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		Subnet string `json:"subnet"`
		SiteID string `json:"site_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	scanID := uuid.New()

	var userID *uuid.UUID
	if uid, err := common.GetUserIDFromContext(r.Context()); err == nil {
		userID = &uid
	}

	subnets := []string{}
	if req.Subnet != "" {
		subnets = append(subnets, req.Subnet)
	}

	var siteID *uuid.UUID
	if req.SiteID != "" {
		parsed, err := uuid.Parse(req.SiteID)
		if err == nil {
			siteID = &parsed
		}
	}

	_, err := g.db.ExecContext(ctx,
		`INSERT INTO discovery_scans (id, site_id, status, methods, subnets, ports, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`,
		scanID, siteID, "pending", pq.Array([]string{"iprange"}), pq.Array(subnets), pq.Array([]int{80, 554, 8080}), userID)
	if err != nil {
		g.logger.Error("Failed to create discovery scan", "error", err)
		jsonError(w, "failed to create scan", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"scan_id": scanID.String(),
	})
}

func (g *Gateway) handleDiscoveryListScans(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	type scanRow struct {
		ID          string     `db:"id"`
		SiteID      string     `db:"site_id"`
		Status      string     `db:"status"`
		Methods     string     `db:"methods"`
		Subnets     string     `db:"subnets"`
		TotalFound  int        `db:"total_found"`
		StartedAt   *time.Time `db:"started_at"`
		CompletedAt *time.Time `db:"completed_at"`
		CreatedAt   time.Time  `db:"created_at"`
	}

	var scans []scanRow
	if err := g.db.SelectContext(ctx, &scans,
		"SELECT id, site_id, status, methods, subnets, total_found, started_at, completed_at, created_at FROM discovery_scans ORDER BY created_at DESC"); err != nil {
		g.logger.Error("Failed to list discovery scans", "error", err)
		jsonError(w, "failed to list scans", http.StatusInternalServerError)
		return
	}

	if scans == nil {
		scans = []scanRow{}
	}

	type scanResp struct {
		ID          string `json:"id"`
		SiteID      string `json:"site_id"`
		Status      string `json:"status"`
		Methods     string `json:"methods"`
		Subnets     string `json:"subnets"`
		TotalFound  int    `json:"total_found"`
		StartedAt   string `json:"started_at"`
		CompletedAt string `json:"completed_at"`
		CreatedAt   string `json:"created_at"`
	}

	result := make([]scanResp, 0, len(scans))
	for _, s := range scans {
		rsp := scanResp{
			ID: s.ID, SiteID: s.SiteID, Status: s.Status,
			Methods: s.Methods, Subnets: s.Subnets, TotalFound: s.TotalFound,
			CreatedAt: s.CreatedAt.Format(time.RFC3339),
		}
		if s.StartedAt != nil {
			rsp.StartedAt = s.StartedAt.Format(time.RFC3339)
		}
		if s.CompletedAt != nil {
			rsp.CompletedAt = s.CompletedAt.Format(time.RFC3339)
		}
		result = append(result, rsp)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"scans": result,
		"total": len(result),
		"page":  1,
		"per_page": len(result),
	})
}

func (g *Gateway) handleDiscoveryGetScan(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	scanID := extractParam(r.URL.Path, "/api/discovery/scans/")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var scan struct {
		ID          string     `db:"id"`
		SiteID      string     `db:"site_id"`
		Status      string     `db:"status"`
		Methods     string     `db:"methods"`
		Subnets     string     `db:"subnets"`
		Ports       string     `db:"ports"`
		TotalFound  int        `db:"total_found"`
		Error       *string    `db:"error"`
		StartedAt   *time.Time `db:"started_at"`
		CompletedAt *time.Time `db:"completed_at"`
		CreatedAt   time.Time  `db:"created_at"`
	}
	if err := g.db.GetContext(ctx, &scan,
		"SELECT id, site_id, status, methods, subnets, ports, total_found, error, started_at, completed_at, created_at FROM discovery_scans WHERE id=$1", scanID); err != nil {
		jsonError(w, "scan not found", http.StatusNotFound)
		return
	}

	resp := map[string]interface{}{
		"id":          scan.ID,
		"site_id":     scan.SiteID,
		"status":      scan.Status,
		"methods":     scan.Methods,
		"subnets":     scan.Subnets,
		"ports":       scan.Ports,
		"total_found": scan.TotalFound,
		"created_at":  scan.CreatedAt.Format(time.RFC3339),
	}
	if scan.StartedAt != nil {
		resp["started_at"] = scan.StartedAt.Format(time.RFC3339)
	} else {
		resp["started_at"] = ""
	}
	if scan.CompletedAt != nil {
		resp["completed_at"] = scan.CompletedAt.Format(time.RFC3339)
	} else {
		resp["completed_at"] = ""
	}
	if scan.Error != nil {
		resp["error"] = *scan.Error
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (g *Gateway) handleDiscoveryGetResults(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/discovery/scans/")
	scanID := strings.TrimSuffix(path, "/results")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	type resultRow struct {
		ID           string            `db:"id"`
		ScanID       string            `db:"scan_id"`
		SiteID       string            `db:"site_id"`
		IPAddress    string            `db:"ip_address"`
		Port         *int              `db:"port"`
		XAddr        *string           `db:"xaddr"`
		Manufacturer *string           `db:"manufacturer"`
		Model        *string           `db:"model"`
		Firmware     *string           `db:"firmware"`
		SerialNumber *string           `db:"serial_number"`
		Hostname     *string           `db:"hostname"`
		Capabilities *string           `db:"capabilities"`
		IsNew        bool              `db:"is_new"`
		AlreadyInDB  bool              `db:"already_in_db"`
		Imported     bool              `db:"imported"`
		CreatedAt    time.Time         `db:"created_at"`
	}

	var rows []resultRow
	if err := g.db.SelectContext(ctx, &rows,
		`SELECT id, scan_id, site_id, ip_address, port, xaddr, manufacturer, model, firmware, serial_number, hostname, capabilities, is_new, already_in_db, imported, created_at
		 FROM discovery_results WHERE scan_id=$1 ORDER BY created_at ASC`,
		scanID); err != nil {
		g.logger.Error("Failed to get discovery results", "error", err)
		jsonError(w, "failed to get results", http.StatusInternalServerError)
		return
	}

	if rows == nil {
		rows = []resultRow{}
	}

	type resultResp struct {
		ID           string                 `json:"id"`
		ScanID       string                 `json:"scan_id"`
		SiteID       string                 `json:"site_id"`
		IPAddress    string                 `json:"ip_address"`
		Port         *int                   `json:"port"`
		XAddr        string                 `json:"xaddr"`
		Manufacturer string                 `json:"manufacturer"`
		Model        string                 `json:"model"`
		Firmware     string                 `json:"firmware"`
		SerialNumber string                 `json:"serial_number"`
		Hostname     string                 `json:"hostname"`
		Capabilities map[string]interface{} `json:"capabilities"`
		IsNew        bool                   `json:"is_new"`
		AlreadyInDB  bool                   `json:"already_in_db"`
		Imported     bool                   `json:"imported"`
		CreatedAt    string                 `json:"created_at"`
	}

	results := make([]resultResp, 0, len(rows))
	for _, r := range rows {
		resp := resultResp{
			ID: r.ID, ScanID: r.ScanID, SiteID: r.SiteID,
			IPAddress: r.IPAddress, Port: r.Port, IsNew: r.IsNew,
			AlreadyInDB: r.AlreadyInDB, Imported: r.Imported,
			CreatedAt: r.CreatedAt.Format(time.RFC3339),
			Capabilities: make(map[string]interface{}),
		}
		if r.XAddr != nil {
			resp.XAddr = *r.XAddr
		}
		if r.Manufacturer != nil {
			resp.Manufacturer = *r.Manufacturer
		}
		if r.Model != nil {
			resp.Model = *r.Model
		}
		if r.Firmware != nil {
			resp.Firmware = *r.Firmware
		}
		if r.SerialNumber != nil {
			resp.SerialNumber = *r.SerialNumber
		}
		if r.Hostname != nil {
			resp.Hostname = *r.Hostname
		}
		if r.Capabilities != nil {
			json.Unmarshal([]byte(*r.Capabilities), &resp.Capabilities)
		}
		results = append(results, resp)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results":  results,
		"total":    len(results),
		"page":     1,
		"per_page": len(results),
	})
}

func (g *Gateway) handleDiscoveryTestCredentials(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		IP       string `json:"ip"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.IP == "" {
		jsonError(w, "ip is required", http.StatusBadRequest)
		return
	}

	if req.Port == 0 {
		req.Port = 80
	}

	deviceURL := "http://" + req.IP + ":" + strconv.Itoa(req.Port) + "/onvif/device_service"
	client := onvif.NewSOAPClient(5*time.Second, &onvif.Credentials{
		Username: req.Username,
		Password: req.Password,
	})

	info, err := onvif.GetDeviceInformation(r.Context(), client, deviceURL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":      false,
			"manufacturer": "",
			"model":        "",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"manufacturer": info.Manufacturer,
		"model":        info.Model,
	})
}

func (g *Gateway) handleDiscoveryImport(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		ScanID   string   `json:"scan_id"`
		Devices  []string `json:"devices"`
		SiteID   string   `json:"site_id"`
		Username string   `json:"username"`
		Password string   `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	created := 0
	failed := 0

	for _, ip := range req.Devices {
		connURL := "rtsp://" + ip + ":554"
		_, err := g.cameraSvc.CreateCamera(ctx, &damv1.CreateCameraRequest{
			SiteId:        req.SiteID,
			Name:          ip,
			ConnectionUrl: connURL,
			OnvifUsername: req.Username,
			OnvifPassword: req.Password,
		})
		if err != nil {
			g.logger.Error("Failed to import camera", "ip", ip, "error", err)
			failed++
		} else {
			created++
			if req.ScanID != "" {
				g.db.ExecContext(ctx,
					"UPDATE discovery_results SET imported=true, imported_at=NOW() WHERE scan_id=$1 AND ip_address=$2",
					req.ScanID, ip)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"created": created,
		"failed":  failed,
	})
}

func (g *Gateway) handleUpdateCameraConfig(w http.ResponseWriter, r *http.Request) {
	cameraID := extractParam(r.URL.Path, "/api/cameras/")
	cameraID = strings.TrimSuffix(cameraID, "/config")

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
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

func (g *Gateway) handleListAllowlist(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}
	var entries []struct {
		ID          string `json:"id" db:"id"`
		CIDR        string `json:"cidr" db:"cidr"`
		Description string `json:"description" db:"description"`
		CreatedAt   string `json:"created_at" db:"created_at"`
	}
	if err := g.db.Select(&entries, "SELECT id, cidr, description, created_at FROM ip_allowlist ORDER BY created_at"); err != nil {
		jsonError(w, "failed to list allowlist", http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []struct {
			ID          string `json:"id" db:"id"`
			CIDR        string `json:"cidr" db:"cidr"`
			Description string `json:"description" db:"description"`
			CreatedAt   string `json:"created_at" db:"created_at"`
		}{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"entries": entries})
}

func (g *Gateway) handleAddAllowlist(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		CIDR        string `json:"cidr"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := g.ipAllowlist.AddCIDR(req.CIDR); err != nil {
		jsonError(w, "invalid CIDR: "+err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := g.db.Exec("INSERT INTO ip_allowlist (cidr, description) VALUES ($1, $2)", req.CIDR, req.Description); err != nil {
		g.ipAllowlist.RemoveCIDR(req.CIDR)
		jsonError(w, "failed to save allowlist entry", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "added"})
}

func (g *Gateway) handleRemoveAllowlist(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}
	cidr := strings.TrimPrefix(r.URL.Path, "/api/admin/allowlist/")
	if cidr == "" {
		jsonError(w, "CIDR required", http.StatusBadRequest)
		return
	}
	g.ipAllowlist.RemoveCIDR(cidr)
	if _, err := g.db.Exec("DELETE FROM ip_allowlist WHERE cidr = $1", cidr); err != nil {
		jsonError(w, "failed to remove allowlist entry", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "removed"})
}

func (g *Gateway) licenseMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if g.licenseClaims != nil && r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/cameras") {
			var count int
			if err := g.db.GetContext(r.Context(), &count, "SELECT COUNT(*) FROM cameras"); err == nil && count >= g.licenseClaims.MaxCameras {
				jsonError(w, "license limit reached: max cameras exceeded", http.StatusForbidden)
				return
			}
		}
		next(w, r)
	}
}

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)
	allowedOrigins := map[string]bool{
		"http://localhost:5173":    true,
		"http://localhost:3000":    true,
		"https://localhost:5173":   true,
		"https://localhost:3000":   true,
	}
	origin := r.Header.Get("Origin")
	if allowedOrigins[origin] {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CSRF-Token")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	path := r.URL.Path

	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
		if path != "/api/login" && path != "/api/csrf-token" && path != "/api/refresh" && !strings.HasPrefix(path, "/api/webhooks") {
			headerToken := r.Header.Get("X-CSRF-Token")
			cookie, err := r.Cookie("csrf_token")
			if err != nil || cookie.Value == "" || subtle.ConstantTimeCompare([]byte(headerToken), []byte(cookie.Value)) != 1 {
				jsonError(w, "CSRF token required", http.StatusForbidden)
				return
			}
		}
	}

	switch {
	case path == "/api/csrf-token":
		g.handleCSRFToken(w, r)
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
	case strings.HasPrefix(path, "/api/cameras/") && strings.HasSuffix(path, "/details") && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameraDetails))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.HasSuffix(path, "/streams") && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameraStreams))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.HasSuffix(path, "/ptz") && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameraPTZ))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.HasSuffix(path, "/network") && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameraNetwork))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.HasSuffix(path, "/diagnostics") && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameraDiagnostics))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.HasSuffix(path, "/recording") && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameraRecording))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.HasSuffix(path, "/onvif") && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameraOnvif))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.HasSuffix(path, "/profiles"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameraControl))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.HasSuffix(path, "/snapshot"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameraControl))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.HasSuffix(path, "/stream-uri"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameraControl))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.HasSuffix(path, "/video-sources"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameraControl))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.HasSuffix(path, "/audio-sources"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameraControl))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.Contains(path, "/device/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameraControl))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.Contains(path, "/imaging/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameraControl))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.Contains(path, "/network/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameraControl))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.Contains(path, "/recording/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameraControl))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.Contains(path, "/analytics/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleCameraControl))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && !strings.Contains(path[len("/api/cameras/"):], "/") && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleGetCamera))(w, r)
	case path == "/api/recordings" && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleRecordings))(w, r)
	case strings.HasPrefix(path, "/api/events"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			g.alertProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/webhooks"):
		g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.notificationProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/playback/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handlePlayback))(w, r)
	case strings.HasPrefix(path, "/api/webrtc/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleWebRTC))(w, r)
	case path == "/api/webrtc/metadata" || strings.HasPrefix(path, "/api/webrtc/metadata"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleWebRTC))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && (strings.Contains(path, "/ptz/") || strings.HasSuffix(path, "/ptz/presets")):
		g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(g.handleCameraControl))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.HasSuffix(path, "/io"):
		g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(g.handleCameraControl))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.HasSuffix(path, "/config") && r.Method == http.MethodPut:
		g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(g.handleUpdateCameraConfig))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && strings.HasSuffix(path, "/credentials") && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(g.handleCameraCredentials))(w, r)
	case strings.HasPrefix(path, "/api/stream/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleStreamURL))(w, r)
	case strings.HasPrefix(path, "/api/thumbnails/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleThumbnails))(w, r)
	case path == "/api/admin/allowlist" && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.ipAllowlist.Middleware(g.requireRole("admin")(g.handleListAllowlist)))(w, r)
	case path == "/api/admin/allowlist" && r.Method == http.MethodPost:
		g.rateLimiter.rateLimitMiddleware(g.ipAllowlist.Middleware(g.requireRole("admin")(g.handleAddAllowlist)))(w, r)
	case strings.HasPrefix(path, "/api/admin/allowlist/") && r.Method == http.MethodDelete:
		g.rateLimiter.rateLimitMiddleware(g.ipAllowlist.Middleware(g.requireRole("admin")(g.handleRemoveAllowlist)))(w, r)
	case strings.HasPrefix(path, "/api/admin/users"):
		g.rateLimiter.rateLimitMiddleware(g.ipAllowlist.Middleware(g.requireRole("admin")(g.handleLogin)))(w, r)
	case path == "/api/sites" && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleListSites))(w, r)
	case path == "/api/sites" && r.Method == http.MethodPost:
		g.rateLimiter.rateLimitMiddleware(g.ipAllowlist.Middleware(g.requireRole("admin")(g.handleCreateSite)))(w, r)
	case strings.HasPrefix(path, "/api/sites/") && r.Method == http.MethodDelete:
		g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(g.handleDeleteSite))(w, r)
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
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.recorderProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/retention-policies"):
		g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.recorderProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/timeline"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.recorderProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/recording-timeline"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.recorderProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/frame-index"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.recorderProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/motion-frames"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.recorderProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/scene-changes"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.recorderProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/audio/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
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
	case strings.HasPrefix(path, "/api/channels"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.notificationProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/templates"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.notificationProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/notification-log"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.notificationProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/admin/config"):
		g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.notificationProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/reports"):
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
			g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(func(w http.ResponseWriter, r *http.Request) {
				r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
				g.reportingProxy.ServeHTTP(w, r)
			}))(w, r)
		} else {
			g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
				r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
				g.reportingProxy.ServeHTTP(w, r)
			}))(w, r)
		}
	case strings.HasPrefix(path, "/api/evidence"):
		if r.Method == http.MethodDelete || r.Method == http.MethodPost || r.Method == http.MethodPut {
			g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(func(w http.ResponseWriter, r *http.Request) {
				r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
				g.exportProxy.ServeHTTP(w, r)
			}))(w, r)
		} else {
			g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
				r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
				g.exportProxy.ServeHTTP(w, r)
			}))(w, r)
		}
	case strings.HasPrefix(path, "/api/incidents"):
		if r.Method == http.MethodDelete {
			g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(func(w http.ResponseWriter, r *http.Request) {
				g.alertProxy.ServeHTTP(w, r)
			}))(w, r)
		} else if r.Method == http.MethodPost || r.Method == http.MethodPut {
			g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(func(w http.ResponseWriter, r *http.Request) {
				g.alertProxy.ServeHTTP(w, r)
			}))(w, r)
		} else {
			g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
				g.alertProxy.ServeHTTP(w, r)
			}))(w, r)
		}
	case strings.HasPrefix(path, "/api/alerts"):
		if r.Method == http.MethodDelete || r.Method == http.MethodPost || r.Method == http.MethodPut {
			g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(func(w http.ResponseWriter, r *http.Request) {
				g.alertProxy.ServeHTTP(w, r)
			}))(w, r)
		} else {
			g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
				g.alertProxy.ServeHTTP(w, r)
			}))(w, r)
		}
	case strings.HasPrefix(path, "/api/rules"):
		if r.Method == http.MethodDelete || r.Method == http.MethodPost || r.Method == http.MethodPut {
			g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(func(w http.ResponseWriter, r *http.Request) {
				g.alertProxy.ServeHTTP(w, r)
			}))(w, r)
		} else {
			g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
				g.alertProxy.ServeHTTP(w, r)
			}))(w, r)
		}
	case strings.HasPrefix(path, "/api/tours"):
		if r.Method == http.MethodDelete || r.Method == http.MethodPost || r.Method == http.MethodPut {
			g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(func(w http.ResponseWriter, r *http.Request) {
				g.alertProxy.ServeHTTP(w, r)
			}))(w, r)
		} else {
			g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
				g.alertProxy.ServeHTTP(w, r)
			}))(w, r)
		}
	case strings.HasPrefix(path, "/api/analytics/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			g.alertProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/intrusion-zones"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			g.alertProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/loitering-zones"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			g.alertProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/abandoned-object-zones"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			g.alertProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/forensics/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
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
	case path == "/api/password/policy":
		g.rateLimiter.rateLimitMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.authProxy.ServeHTTP(w, r)
		})(w, r)
	case path == "/api/password/change":
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.authProxy.ServeHTTP(w, r)
		}))(w, r)
	case path == "/api/refresh":
		g.rateLimiter.rateLimitMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.authProxy.ServeHTTP(w, r)
		})(w, r)
	case strings.HasPrefix(path, "/api/mfa/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.authProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/sessions"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.authProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/api-keys"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.authProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/sso/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.authProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/pos/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			g.posProxy.ServeHTTP(w, r)
		}))(w, r)
	case path == "/api/discovery/scan" && r.Method == http.MethodPost:
		g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(g.handleDiscoveryScan))(w, r)
	case path == "/api/discovery/scans" && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleDiscoveryListScans))(w, r)
	case strings.HasPrefix(path, "/api/discovery/scans/") && strings.HasSuffix(path, "/results") && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleDiscoveryGetResults))(w, r)
	case strings.HasPrefix(path, "/api/discovery/scans/") && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleDiscoveryGetScan))(w, r)
	case path == "/api/discovery/test-credentials" && r.Method == http.MethodPost:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleDiscoveryTestCredentials))(w, r)
	case path == "/api/discovery/import" && r.Method == http.MethodPost:
		g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(g.handleDiscoveryImport))(w, r)
	case strings.HasPrefix(path, "/api/discovery/scans") && r.Method == http.MethodPost:
		g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(g.handleDiscovery))(w, r)
	case strings.HasPrefix(path, "/api/discovery/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleDiscovery))(w, r)
	case strings.HasPrefix(path, "/api/federation/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.federationProxy.ServeHTTP(w, r)
		}))(w, r)
	case strings.HasPrefix(path, "/api/onvif-events/"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleOnvifEvents))(w, r)
	case path == "/api/cameras" && r.Method == http.MethodPost:
		g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(g.handleCreateCamera))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && !strings.Contains(path[len("/api/cameras/"):], "/") && r.Method == http.MethodPut:
		g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(g.handleUpdateCamera))(w, r)
	case strings.HasPrefix(path, "/api/cameras/") && !strings.Contains(path[len("/api/cameras/"):], "/") && r.Method == http.MethodDelete:
		g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(g.handleDeleteCamera))(w, r)
	case strings.HasPrefix(path, "/api/models"):
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			g.modelRegistryProxy.ServeHTTP(w, r)
		}))(w, r)
	case path == "/api/plugins" && r.Method == http.MethodGet:
		g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleListPlugins))(w, r)
	case path == "/api/plugins" && r.Method == http.MethodPost:
		g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(g.handleRegisterPlugin))(w, r)
	case strings.HasPrefix(path, "/api/plugins/") && r.Method == http.MethodPut:
		g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(g.handleUpdatePlugin))(w, r)
	case strings.HasPrefix(path, "/api/plugins/") && r.Method == http.MethodDelete:
		g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(g.handleDeletePlugin))(w, r)
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

type Plugin struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	Status      string   `json:"status"`
	Endpoint    string   `json:"endpoint"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

func (g *Gateway) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var plugins []Plugin
	if err := g.db.SelectContext(ctx, &plugins,
		"SELECT id, name, version, description, permissions, status, endpoint, created_at, updated_at FROM plugins ORDER BY name"); err != nil {
		g.logger.Error("Failed to list plugins", "error", err)
		jsonError(w, "failed to list plugins", http.StatusInternalServerError)
		return
	}
	if plugins == nil {
		plugins = []Plugin{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"plugins": plugins})
}

func (g *Gateway) handleRegisterPlugin(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		Name        string   `json:"name"`
		Version     string   `json:"version"`
		Description string   `json:"description"`
		Endpoint    string   `json:"endpoint"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var plugin Plugin
	err := g.db.GetContext(ctx, &plugin,
		`INSERT INTO plugins (name, version, description, endpoint, permissions, status)
		 VALUES ($1, $2, $3, $4, $5, 'disabled')
		 RETURNING id, name, version, description, permissions, status, endpoint, created_at, updated_at`,
		req.Name, req.Version, req.Description, req.Endpoint, req.Permissions)
	if err != nil {
		g.logger.Error("Failed to register plugin", "error", err)
		jsonError(w, "failed to register plugin", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(plugin)
}

func (g *Gateway) handleUpdatePlugin(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}
	pluginID := extractParam(r.URL.Path, "/api/plugins/")
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Status != "enabled" && req.Status != "disabled" && req.Status != "error" {
		jsonError(w, "status must be 'enabled', 'disabled', or 'error'", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var plugin Plugin
	err := g.db.GetContext(ctx, &plugin,
		`UPDATE plugins SET status=$1, updated_at=NOW()
		 WHERE id=$2
		 RETURNING id, name, version, description, permissions, status, endpoint, created_at, updated_at`,
		req.Status, pluginID)
	if err != nil {
		g.logger.Error("Failed to update plugin", "error", err)
		jsonError(w, "plugin not found", http.StatusNotFound)
		return
	}
	if g.nc != nil && req.Status == "enabled" {
		subj := fmt.Sprintf("plugins.%s.enabled", plugin.Name)
		g.nc.Publish(subj, []byte(`{"status":"enabled"}`))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plugin)
}

func (g *Gateway) handleDeletePlugin(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}
	pluginID := extractParam(r.URL.Path, "/api/plugins/")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	result, err := g.db.ExecContext(ctx, "DELETE FROM plugins WHERE id=$1", pluginID)
	if err != nil {
		g.logger.Error("Failed to delete plugin", "error", err)
		jsonError(w, "failed to delete plugin", http.StatusInternalServerError)
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		jsonError(w, "plugin not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
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
	logger := common.NewLogger("api-gateway")
	slog.SetDefault(logger)

	common.CheckJWTSecret()

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
