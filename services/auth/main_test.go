package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
