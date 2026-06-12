package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testDummyURL() *url.URL {
	u, _ := url.Parse("http://127.0.0.1:1")
	return u
}

var testGRPCConn *grpc.ClientConn

func init() {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	srv := grpc.NewServer()
	go srv.Serve(lis)
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		panic(err)
	}
	testGRPCConn = conn
}

func testGateway() *Gateway {
	cc := testGRPCConn
	return &Gateway{
		logger:             testLogger(),
		rateLimiter:        newRateLimiter(1000, 2000, 0),
		cameraCC:           cc,
		authProxy:          httputil.NewSingleHostReverseProxy(testDummyURL()),
		playbackProxy:      httputil.NewSingleHostReverseProxy(testDummyURL()),
		webrtcProxy:        httputil.NewSingleHostReverseProxy(testDummyURL()),
		cameraControlProxy: httputil.NewSingleHostReverseProxy(testDummyURL()),
		thumbnailsProxy:    httputil.NewSingleHostReverseProxy(testDummyURL()),
		recorderProxy:      httputil.NewSingleHostReverseProxy(testDummyURL()),
		exportProxy:        httputil.NewSingleHostReverseProxy(testDummyURL()),
		alertProxy:         httputil.NewSingleHostReverseProxy(testDummyURL()),
		auditProxy:         httputil.NewSingleHostReverseProxy(testDummyURL()),
		posProxy:           httputil.NewSingleHostReverseProxy(testDummyURL()),
		discoveryProxy:     httputil.NewSingleHostReverseProxy(testDummyURL()),
		federationProxy:    httputil.NewSingleHostReverseProxy(testDummyURL()),
		onvifEventsProxy:   httputil.NewSingleHostReverseProxy(testDummyURL()),
		notificationProxy:  httputil.NewSingleHostReverseProxy(testDummyURL()),
		reportingProxy:     httputil.NewSingleHostReverseProxy(testDummyURL()),
		modelRegistryProxy: httputil.NewSingleHostReverseProxy(testDummyURL()),
	}
}

func generateTestJWT(username, role string) string {
	claims := &common.Claims{
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte("test-secret"))
	return signed
}

func TestSetSecurityHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	setSecurityHeaders(w)

	h := w.Header()
	tests := []struct {
		name  string
		key   string
		want  string
	}{
		{"X-Content-Type-Options", "X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "X-Frame-Options", "DENY"},
		{"Referrer-Policy", "Referrer-Policy", "no-referrer"},
		{"Permissions-Policy", "Permissions-Policy", "camera=(), microphone=(), geolocation=()"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := h.Get(tt.key); got != tt.want {
				t.Errorf("header %s = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestGenerateCSRFToken(t *testing.T) {
	t1 := generateCSRFToken()
	t2 := generateCSRFToken()
	if t1 == "" {
		t.Error("expected non-empty token")
	}
	if t1 == t2 {
		t.Error("expected unique tokens")
	}
	if len(t1) != 64 {
		t.Errorf("expected 64 hex chars (32 bytes), got %d", len(t1))
	}
}

func TestHandleCSRFToken(t *testing.T) {
	g := testGateway()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/csrf-token", nil)
	g.handleCSRFToken(w, r)

	resp := w.Result()
	defer resp.Body.Close()

	// Check cookie
	cookies := resp.Cookies()
	var csrfCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "csrf_token" {
			csrfCookie = c
			break
		}
	}
	if csrfCookie == nil {
		t.Fatal("expected csrf_token cookie")
	}
	if csrfCookie.Value == "" {
		t.Error("expected non-empty cookie value")
	}
	if r.TLS != nil && !csrfCookie.Secure {
		t.Error("expected Secure flag on cookie for HTTPS")
	}
	if r.TLS == nil && csrfCookie.Secure {
		t.Error("expected no Secure flag on cookie for HTTP")
	}
	if csrfCookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("expected SameSite=Strict, got %v", csrfCookie.SameSite)
	}
	if csrfCookie.HttpOnly {
		t.Error("expected HttpOnly=false for CSRF cookie")
	}

	// Check JSON body
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal("expected JSON body:", err)
	}
	if body["csrf_token"] != csrfCookie.Value {
		t.Error("JSON token and cookie value should match")
	}

	// Check content type
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestAuthMiddleware(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret")
	common.ReloadJWTKey()
	defer func() {
		os.Unsetenv("JWT_SECRET")
		common.ReloadJWTKey()
	}()

	g := testGateway()
	handlerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		if r.Context().Value(common.UserKey) != "testuser" {
			t.Errorf("UserKey = %v, want testuser", r.Context().Value(common.UserKey))
		}
		if r.Context().Value(common.RoleKey) != "admin" {
			t.Errorf("RoleKey = %v, want admin", r.Context().Value(common.RoleKey))
		}
		w.WriteHeader(http.StatusOK)
	})

	t.Run("valid token", func(t *testing.T) {
		handlerCalled = false
		token := generateTestJWT("testuser", "admin")
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		r.Header.Set("Authorization", "Bearer "+token)

		g.authMiddleware(inner)(w, r)
		if !handlerCalled {
			t.Error("inner handler should be called")
		}
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
	})

	t.Run("no token", func(t *testing.T) {
		handlerCalled = false
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/test", nil)

		g.authMiddleware(inner)(w, r)
		if handlerCalled {
			t.Error("inner handler should not be called")
		}
		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", w.Code)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		handlerCalled = false
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		r.Header.Set("Authorization", "Bearer invalid-token")

		g.authMiddleware(inner)(w, r)
		if handlerCalled {
			t.Error("inner handler should not be called")
		}
		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", w.Code)
		}
	})
}

func TestRequireRole(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret")
	common.ReloadJWTKey()
	defer func() {
		os.Unsetenv("JWT_SECRET")
		common.ReloadJWTKey()
	}()

	g := testGateway()
	handlerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	makeReq := func(role string) *http.Request {
		token := generateTestJWT("user", role)
		r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		return r
	}

	t.Run("admin allowed for admin route", func(t *testing.T) {
		handlerCalled = false
		w := httptest.NewRecorder()
		g.requireRole("admin")(inner)(w, makeReq("admin"))
		if !handlerCalled {
			t.Error("admin should be allowed on admin route")
		}
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
	})

	t.Run("operator blocked from admin route", func(t *testing.T) {
		handlerCalled = false
		w := httptest.NewRecorder()
		g.requireRole("admin")(inner)(w, makeReq("operator"))
		if handlerCalled {
			t.Error("operator should not be allowed on admin route")
		}
		if w.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", w.Code)
		}
	})

	t.Run("viewer blocked from admin route", func(t *testing.T) {
		handlerCalled = false
		w := httptest.NewRecorder()
		g.requireRole("admin")(inner)(w, makeReq("viewer"))
		if handlerCalled {
			t.Error("viewer should not be allowed on admin route")
		}
		if w.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", w.Code)
		}
	})

	t.Run("operator allowed for operator route", func(t *testing.T) {
		handlerCalled = false
		w := httptest.NewRecorder()
		g.requireRole("operator")(inner)(w, makeReq("operator"))
		if !handlerCalled {
			t.Error("operator should be allowed on operator route")
		}
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
	})

	t.Run("viewer blocked from operator route", func(t *testing.T) {
		handlerCalled = false
		w := httptest.NewRecorder()
		g.requireRole("operator")(inner)(w, makeReq("viewer"))
		if handlerCalled {
			t.Error("viewer should not be allowed on operator route")
		}
		if w.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", w.Code)
		}
	})

	t.Run("viewer allowed for viewer route", func(t *testing.T) {
		handlerCalled = false
		w := httptest.NewRecorder()
		g.requireRole("viewer")(inner)(w, makeReq("viewer"))
		if !handlerCalled {
			t.Error("viewer should be allowed on viewer route")
		}
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
	})

	t.Run("no token", func(t *testing.T) {
		handlerCalled = false
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		g.requireRole("admin")(inner)(w, r)
		if handlerCalled {
			t.Error("inner handler should not be called")
		}
		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", w.Code)
		}
	})
}

func TestCSRFMiddleware(t *testing.T) {
	g := testGateway()
	handlerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	t.Run("GET bypasses CSRF", func(t *testing.T) {
		handlerCalled = false
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		g.csrfMiddleware(inner)(w, r)
		if !handlerCalled {
			t.Error("GET should bypass CSRF check")
		}
	})

	t.Run("HEAD bypasses CSRF", func(t *testing.T) {
		handlerCalled = false
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodHead, "/api/test", nil)
		g.csrfMiddleware(inner)(w, r)
		if !handlerCalled {
			t.Error("HEAD should bypass CSRF check")
		}
	})

	t.Run("OPTIONS bypasses CSRF", func(t *testing.T) {
		handlerCalled = false
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodOptions, "/api/test", nil)
		g.csrfMiddleware(inner)(w, r)
		if !handlerCalled {
			t.Error("OPTIONS should bypass CSRF check")
		}
	})

	t.Run("POST without CSRF token returns 403", func(t *testing.T) {
		handlerCalled = false
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/test", nil)
		g.csrfMiddleware(inner)(w, r)
		if handlerCalled {
			t.Error("inner handler should not be called")
		}
		if w.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", w.Code)
		}
	})

	t.Run("POST with valid CSRF token", func(t *testing.T) {
		handlerCalled = false
		token := generateCSRFToken()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/test", nil)
		r.Header.Set("X-CSRF-Token", token)
		r.AddCookie(&http.Cookie{Name: "csrf_token", Value: token})
		g.csrfMiddleware(inner)(w, r)
		if !handlerCalled {
			t.Error("inner handler should be called with valid CSRF token")
		}
	})

	t.Run("POST with mismatched CSRF token", func(t *testing.T) {
		handlerCalled = false
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/test", nil)
		r.Header.Set("X-CSRF-Token", "wrong-token")
		r.AddCookie(&http.Cookie{Name: "csrf_token", Value: "cookie-token"})
		g.csrfMiddleware(inner)(w, r)
		if handlerCalled {
			t.Error("inner handler should not be called")
		}
		if w.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", w.Code)
		}
	})
}

func TestCORS_AllowedOrigins(t *testing.T) {
	g := testGateway()
	allowed := []string{
		"http://localhost:5173",
		"http://localhost:3000",
		"https://localhost:5173",
		"https://localhost:3000",
	}
	disallowed := []string{
		"http://evil.com",
		"https://malicious.site",
		"http://localhost:9999",
		"",
	}

	for _, origin := range allowed {
		t.Run("allowed origin: "+origin, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
			r.Header.Set("Origin", origin)
			g.ServeHTTP(w, r)

			got := w.Header().Get("Access-Control-Allow-Origin")
			if got != origin {
				t.Errorf("ACAO = %q, want %q", got, origin)
			}
		})
	}

	for _, origin := range disallowed {
		t.Run("disallowed origin: "+origin, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
			if origin != "" {
				r.Header.Set("Origin", origin)
			}
			g.ServeHTTP(w, r)

			got := w.Header().Get("Access-Control-Allow-Origin")
			if got != "" {
				t.Errorf("ACAO = %q, want empty for disallowed origin", got)
			}
		})
	}
}

func TestCORS_OptionsPreflight(t *testing.T) {
	g := testGateway()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
	r.Header.Set("Origin", "http://localhost:5173")
	g.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for OPTIONS preflight", w.Code)
	}

	h := w.Header()
	if h.Get("Access-Control-Allow-Methods") == "" {
		t.Error("expected Access-Control-Allow-Methods header")
	}
	if h.Get("Access-Control-Allow-Headers") == "" {
		t.Error("expected Access-Control-Allow-Headers header")
	}
	if h.Get("Access-Control-Allow-Credentials") != "true" {
		t.Error("expected Access-Control-Allow-Credentials: true")
	}
}

func TestSecurityHeadersInServeHTTP(t *testing.T) {
	g := testGateway()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	g.ServeHTTP(w, r)

	h := w.Header()
	if h.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("expected X-Content-Type-Options: nosniff")
	}
	if h.Get("X-Frame-Options") != "DENY" {
		t.Error("expected X-Frame-Options: DENY")
	}
}

func TestEndpoint_CSRFToken(t *testing.T) {
	g := testGateway()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/csrf-token", nil)
	g.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestEndpoint_Health(t *testing.T) {
	g := testGateway()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	g.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestEndpoint_Ready(t *testing.T) {
	g := testGateway()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/ready", nil)
	g.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestEndpoint_EvidenceAdminOnly(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret")
	common.ReloadJWTKey()
	defer func() {
		os.Unsetenv("JWT_SECRET")
		common.ReloadJWTKey()
	}()

	g := testGateway()

	tests := []struct {
		name   string
		method string
	}{
		{"POST", http.MethodPost},
		{"PUT", http.MethodPut},
		{"DELETE", http.MethodDelete},
	}

	for _, tt := range tests {
		t.Run(tt.name+" admin allowed", func(t *testing.T) {
			token := generateTestJWT("admin", "admin")
			w := httptest.NewRecorder()
			r := httptest.NewRequest(tt.method, "/api/evidence/123", strings.NewReader(`{}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("Origin", "http://localhost:5173")
			// Set CSRF token (evidence route requires it)
			csrf := generateCSRFToken()
			r.Header.Set("X-CSRF-Token", csrf)
			r.AddCookie(&http.Cookie{Name: "csrf_token", Value: csrf})
			g.ServeHTTP(w, r)

			// We expect a proxy attempt (502 because backends aren't real) rather than 403
			if w.Code == http.StatusForbidden || w.Code == http.StatusUnauthorized {
				t.Errorf("admin got %d, expected proxy attempt (502/404)", w.Code)
			}
		})
	}

	for _, tt := range tests {
		t.Run(tt.name+" viewer blocked", func(t *testing.T) {
			token := generateTestJWT("viewer", "viewer")
			w := httptest.NewRecorder()
			r := httptest.NewRequest(tt.method, "/api/evidence/123", strings.NewReader(`{}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("Origin", "http://localhost:5173")
			csrf := generateCSRFToken()
			r.Header.Set("X-CSRF-Token", csrf)
			r.AddCookie(&http.Cookie{Name: "csrf_token", Value: csrf})
			g.ServeHTTP(w, r)

			if w.Code != http.StatusForbidden {
				t.Errorf("viewer got %d, want 403", w.Code)
			}
		})
	}
}

func TestEndpoint_WebhooksAdminOnly(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret")
	common.ReloadJWTKey()
	defer func() {
		os.Unsetenv("JWT_SECRET")
		common.ReloadJWTKey()
	}()

	g := testGateway()

	t.Run("admin allowed", func(t *testing.T) {
		token := generateTestJWT("admin", "admin")
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/webhooks", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Origin", "http://localhost:5173")
		g.ServeHTTP(w, r)

		if w.Code == http.StatusForbidden || w.Code == http.StatusUnauthorized {
			t.Errorf("admin got %d, expected proxy attempt", w.Code)
		}
	})

	t.Run("viewer blocked", func(t *testing.T) {
		token := generateTestJWT("viewer", "viewer")
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/webhooks", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Origin", "http://localhost:5173")
		g.ServeHTTP(w, r)

		if w.Code != http.StatusForbidden {
			t.Errorf("viewer got %d, want 403", w.Code)
		}
	})
}

func TestEndpoint_SiteDeleteAdminOnly(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret")
	common.ReloadJWTKey()
	defer func() {
		os.Unsetenv("JWT_SECRET")
		common.ReloadJWTKey()
	}()

	g := testGateway()

	t.Run("admin allowed", func(t *testing.T) {
		token := generateTestJWT("admin", "admin")
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/sites/site-123", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Origin", "http://localhost:5173")
		csrf := generateCSRFToken()
		r.Header.Set("X-CSRF-Token", csrf)
		r.AddCookie(&http.Cookie{Name: "csrf_token", Value: csrf})
		g.ServeHTTP(w, r)

		if w.Code == http.StatusForbidden || w.Code == http.StatusUnauthorized {
			t.Errorf("admin got %d, expected proxy attempt", w.Code)
		}
	})

	t.Run("viewer blocked", func(t *testing.T) {
		token := generateTestJWT("viewer", "viewer")
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/sites/site-123", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Origin", "http://localhost:5173")
		csrf := generateCSRFToken()
		r.Header.Set("X-CSRF-Token", csrf)
		r.AddCookie(&http.Cookie{Name: "csrf_token", Value: csrf})
		g.ServeHTTP(w, r)

		if w.Code != http.StatusForbidden {
			t.Errorf("viewer got %d, want 403", w.Code)
		}
	})
}

func TestEndpoint_EvidenceGETauthOnly(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret")
	common.ReloadJWTKey()
	defer func() {
		os.Unsetenv("JWT_SECRET")
		common.ReloadJWTKey()
	}()

	g := testGateway()

	t.Run("viewer allowed for GET evidence", func(t *testing.T) {
		token := generateTestJWT("viewer", "viewer")
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/evidence/123", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Origin", "http://localhost:5173")
		g.ServeHTTP(w, r)

		if w.Code == http.StatusForbidden || w.Code == http.StatusUnauthorized {
			t.Errorf("viewer got %d, expected proxy attempt for GET evidence", w.Code)
		}
	})

	t.Run("unauthenticated blocked for GET evidence", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/evidence/123", nil)
		r.Header.Set("Origin", "http://localhost:5173")
		g.ServeHTTP(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("unauthenticated got %d, want 401", w.Code)
		}
	})
}

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter(10, 5, 0)

	// First requests should pass
	for i := 0; i < 5; i++ {
		if !rl.Allow("127.0.0.1") {
			t.Errorf("request %d should be allowed (burst)", i)
		}
	}

	// 6th request should be rate limited
	if rl.Allow("127.0.0.1") {
		t.Error("should be rate limited after burst")
	}

	// Different IP should pass
	if !rl.Allow("192.168.1.1") {
		t.Error("different IP should be allowed")
	}
}

func TestHandleDeleteSite_MissingID(t *testing.T) {
	g := testGateway()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/sites/", nil)
	g.handleDeleteSite(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}

	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["error"] != "site ID required" {
		t.Errorf("error = %q, want 'site ID required'", body["error"])
	}
}

func TestHandlePlayback_TenantBlockedNoDB(t *testing.T) {
	// With no DB, the tenant check should block empty tenantID
	g := testGateway()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/playback/cam1/file.mp4", nil)
	g.handlePlayback(w, r)

	// Should return 403 when tenantID is empty (tenant isolation)
	if w.Code != http.StatusForbidden {
		t.Errorf("playback should return 403 when tenantID is empty, got %d", w.Code)
	}
}
