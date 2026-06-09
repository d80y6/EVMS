package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/dam-vms/dam/pkg/common"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultAuthConfig()
	if config.HTTPAddr != ":8081" {
		t.Errorf("default HTTPAddr = %q, want %q", config.HTTPAddr, ":8081")
	}
	if config.TokenExpiry == 0 {
		t.Error("default TokenExpiry should not be zero")
	}
}

func TestConfigValidation(t *testing.T) {
	config := AuthConfig{JWTSecret: nil}
	if err := config.Validate(); err == nil {
		t.Error("expected validation error with empty JWTSecret")
	}

	config.JWTSecret = []byte("valid-secret")
	if err := config.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

func withUserContext(r *http.Request, username string) *http.Request {
	ctx := context.WithValue(r.Context(), common.UserKey, username)
	return r.WithContext(ctx)
}

func TestIPRateLimiter_FirstRequestAllowed(t *testing.T) {
	rl := newIPRateLimiter(5, 1*time.Minute)
	if !rl.Allow("192.168.1.1") {
		t.Error("expected first request to be allowed")
	}
}

func TestIPRateLimiter_ExceedsLimit(t *testing.T) {
	rl := newIPRateLimiter(3, 1*time.Minute)

	for i := 0; i < 3; i++ {
		if !rl.Allow("10.0.0.1") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
	if rl.Allow("10.0.0.1") {
		t.Error("expected request after limit to be denied")
	}
}

func TestIPRateLimiter_DifferentIPs(t *testing.T) {
	rl := newIPRateLimiter(2, 1*time.Minute)

	rl.Allow("192.168.1.1")
	rl.Allow("192.168.1.1")

	if rl.Allow("192.168.1.1") {
		t.Error("expected third request from same IP to be denied")
	}
	if !rl.Allow("10.0.0.2") {
		t.Error("expected request from different IP to be allowed")
	}
}

func TestIPRateLimiter_WindowExpiry(t *testing.T) {
	rl := newIPRateLimiter(2, 50*time.Millisecond)

	if !rl.Allow("10.0.0.1") {
		t.Error("expected first request to be allowed")
	}
	if !rl.Allow("10.0.0.1") {
		t.Error("expected second request to be allowed")
	}
	if rl.Allow("10.0.0.1") {
		t.Error("expected third request to be denied")
	}

	time.Sleep(60 * time.Millisecond)

	if !rl.Allow("10.0.0.1") {
		t.Error("expected request after window expiry to be allowed")
	}
}

func TestIPRateLimiter_ConcurrentSafe(t *testing.T) {
	rl := newIPRateLimiter(100, 1*time.Minute)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.Allow("10.0.0.1")
		}()
	}
	wg.Wait()
	// If we got here without race, the test passes
}

func TestHandleLogout_MethodNotAllowed(t *testing.T) {
	req := withUserContext(httptest.NewRequest(http.MethodGet, "/auth/logout", nil), "testuser")
	rr := httptest.NewRecorder()

	s := &AuthService{}
	s.handleLogout(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "method not allowed" {
		t.Errorf("error = %q, want %q", resp["error"], "method not allowed")
	}
}

func TestHandleLogout_EmptyBody(t *testing.T) {
	req := withUserContext(httptest.NewRequest(http.MethodPost, "/auth/logout", bytes.NewReader(nil)), "testuser")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s := &AuthService{}
	s.handleLogout(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "invalid request body" {
		t.Errorf("error = %q, want %q", resp["error"], "invalid request body")
	}
}

func TestHandleLogout_MissingRefreshToken(t *testing.T) {
	body, _ := json.Marshal(map[string]string{})
	req := withUserContext(httptest.NewRequest(http.MethodPost, "/auth/logout", bytes.NewReader(body)), "testuser")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s := &AuthService{}
	s.handleLogout(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "refresh_token required" {
		t.Errorf("error = %q, want %q", resp["error"], "refresh_token required")
	}
}

func TestOIDCCallback_MissingState(t *testing.T) {
	s := &AuthService{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	req := httptest.NewRequest(http.MethodGet, "/auth/sso/test/callback?code=abc123", nil)
	rr := httptest.NewRecorder()

	provider := SSOProvider{Name: "test", ProviderType: "oidc"}
	s.handleOIDCCallback(rr, req, provider)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "state parameter required" {
		t.Errorf("error = %q, want %q", resp["error"], "state parameter required")
	}
}

func TestOIDCCallback_MissingStateCookie(t *testing.T) {
	s := &AuthService{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	req := httptest.NewRequest(http.MethodGet, "/auth/sso/test/callback?code=abc123&state=xyz789", nil)
	rr := httptest.NewRecorder()

	provider := SSOProvider{Name: "test", ProviderType: "oidc"}
	s.handleOIDCCallback(rr, req, provider)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "invalid state" {
		t.Errorf("error = %q, want %q", resp["error"], "invalid state")
	}
}

func TestOIDCCallback_StateCookieMismatch(t *testing.T) {
	s := &AuthService{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	req := httptest.NewRequest(http.MethodGet, "/auth/sso/test/callback?code=abc123&state=from_query", nil)
	req.AddCookie(&http.Cookie{Name: "oidc_state", Value: "from_cookie"})
	rr := httptest.NewRecorder()

	provider := SSOProvider{Name: "test", ProviderType: "oidc"}
	s.handleOIDCCallback(rr, req, provider)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "invalid state" {
		t.Errorf("error = %q, want %q", resp["error"], "invalid state")
	}
}

func TestExtractNameIDFromSAML_WithNamespace(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<saml2:Response xmlns:saml2="urn:oasis:names:tc:SAML:2.0:protocol"
    xmlns:saml2a="urn:oasis:names:tc:SAML:2.0:assertion"
    Destination="https://example.com/acs">
    <saml2a:Assertion>
        <saml2a:Subject>
            <saml2a:NameID Format="urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress">
                user@example.com
            </saml2a:NameID>
        </saml2a:Subject>
    </saml2a:Assertion>
</saml2:Response>`
	result := extractNameIDFromSAML(xml)
	if result != "user@example.com" {
		t.Errorf("got %q, want %q", result, "user@example.com")
	}
}

func TestExtractNameIDFromSAML_WithoutNamespace(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Assertion>
        <Subject>
            <NameID>simple-user@example.com</NameID>
        </Subject>
    </Assertion>
</Response>`
	result := extractNameIDFromSAML(xml)
	if result != "simple-user@example.com" {
		t.Errorf("got %q, want %q", result, "simple-user@example.com")
	}
}

func TestExtractNameIDFromSAML_NoNameID(t *testing.T) {
	xml := `<Response><Assertion><Subject></Subject></Assertion></Response>`
	result := extractNameIDFromSAML(xml)
	if result != "" {
		t.Errorf("got %q, want empty string", result)
	}
}

func TestExtractNameIDFromSAML_InvalidXML(t *testing.T) {
	result := extractNameIDFromSAML("not xml at all")
	if result != "" {
		t.Errorf("got %q, want empty string", result)
	}
}

func TestExtractNameIDFromSAML_EmptyString(t *testing.T) {
	result := extractNameIDFromSAML("")
	if result != "" {
		t.Errorf("got %q, want empty string", result)
	}
}

func TestHandleLogin_RateLimited(t *testing.T) {
	rl := newIPRateLimiter(1, 1*time.Minute)
	s := &AuthService{loginRateLimiter: rl, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	// Exhaust the rate limit for this IP
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader([]byte(`{"username":"u","password":"p"}`)))
	req.RemoteAddr = "203.0.113.1:12345"
	rr := httptest.NewRecorder()
	rl.Allow("203.0.113.1:12345") // consume the single allowed request

	s.handleLogin(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusTooManyRequests)
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "too many login attempts" {
		t.Errorf("error = %q, want %q", resp["error"], "too many login attempts")
	}
}

func TestHandleLogout_InvalidJSON(t *testing.T) {
	req := withUserContext(httptest.NewRequest(http.MethodPost, "/auth/logout", bytes.NewReader([]byte("{invalid"))), "testuser")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s := &AuthService{}
	s.handleLogout(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "invalid request body" {
		t.Errorf("error = %q, want %q", resp["error"], "invalid request body")
	}
}
