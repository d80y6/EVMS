package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestFederationService() *FederationService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &FederationService{
		config: &FederationConfig{
			Port:  ":8099",
			DBURL: "",
		},
		logger: logger,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func TestDefaultFederationConfig(t *testing.T) {
	cfg := DefaultFederationConfig()
	assert.Equal(t, ":8099", cfg.Port)
	assert.Equal(t, "", cfg.DBURL)
}

func TestDefaultFederationConfig_EnvOverride(t *testing.T) {
	os.Setenv("FEDERATION_PORT", ":9099")
	os.Setenv("DB_URL", "postgres://localhost:5432/test")
	defer func() {
		os.Unsetenv("FEDERATION_PORT")
		os.Unsetenv("DB_URL")
	}()
	cfg := DefaultFederationConfig()
	assert.Equal(t, ":9099", cfg.Port)
	assert.Equal(t, "postgres://localhost:5432/test", cfg.DBURL)
}

func TestJSONError(t *testing.T) {
	rr := httptest.NewRecorder()
	jsonError(rr, "not found", http.StatusNotFound)
	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	assert.Equal(t, "not found", body["error"])
}

func TestJSONOK(t *testing.T) {
	rr := httptest.NewRecorder()
	jsonOK(rr, map[string]string{"key": "value"})
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	assert.Equal(t, "value", body["key"])
}

func TestHandleSites_NoDB(t *testing.T) {
	s := newTestFederationService()

	t.Run("GET", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/federation/sites", nil)
		s.handleSites(rr, req)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		var body map[string]string
		json.Unmarshal(rr.Body.Bytes(), &body)
		assert.Equal(t, "database not configured", body["error"])
	})

	t.Run("POST", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/federation/sites", bytes.NewReader([]byte(`{"name":"test","url":"http://example.com"}`)))
		s.handleSites(rr, req)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})

	t.Run("unsupported method", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/federation/sites", nil)
		s.handleSites(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestHandleSites_POST_InvalidJSON_NoDB(t *testing.T) {
	s := newTestFederationService()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/federation/sites", bytes.NewReader([]byte(`not-json`)))
	s.handleSites(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandleSites_POST_DBRequired(t *testing.T) {
	s := newTestFederationService()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/federation/sites", bytes.NewReader([]byte(`{"name":"test","url":"http://example.com"}`)))
	s.handleSites(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandleSiteByID_NoID(t *testing.T) {
	s := newTestFederationService()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/federation/sites/", nil)
	s.handleSiteByID(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	assert.Equal(t, "site id required", body["error"])
}

func TestHandleSiteByID_Routing(t *testing.T) {
	s := newTestFederationService()
	id := "some-id"

	tests := []struct {
		name   string
		method string
	}{
		{"GET", http.MethodGet},
		{"PUT", http.MethodPut},
		{"DELETE", http.MethodDelete},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			path := "/api/federation/sites/" + id
			req := httptest.NewRequest(tt.method, path, bytes.NewReader([]byte(`{}`)))
			s.handleSiteByID(rr, req)
			assert.Equal(t, http.StatusInternalServerError, rr.Code)
		})
	}

	t.Run("unsupported method", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/federation/sites/"+id, nil)
		s.handleSiteByID(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestHandleSearch_NoDB(t *testing.T) {
	s := newTestFederationService()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/federation/search?camera_id=cam-1", nil)
	s.handleSearch(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandlePlaybackProxy_NoDB(t *testing.T) {
	s := newTestFederationService()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/federation/playback/site-id/some-path", nil)
	s.handlePlaybackProxy(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandlePlaybackProxy_NoSiteID(t *testing.T) {
	s := newTestFederationService()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/federation/playback/", nil)
	s.handlePlaybackProxy(rr, req)
	// DB check happens before site ID validation, so returns 500
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestMeasureLatency_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	s := newTestFederationService()
	latency := s.measureLatency(server.URL)
	assert.GreaterOrEqual(t, latency, 0)
}

func TestMeasureLatency_Failure(t *testing.T) {
	s := newTestFederationService()
	latency := s.measureLatency("http://localhost:19999")
	assert.Equal(t, -1, latency)
}

func TestNewFederationService_NoDB(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	config := &FederationConfig{Port: ":8099", DBURL: ""}
	svc, err := NewFederationService(config, logger)
	require.NoError(t, err)
	assert.NotNil(t, svc)
	assert.Nil(t, svc.db)
	assert.Equal(t, ":8099", svc.config.Port)
}

func TestNewFederationService_NilLogger(t *testing.T) {
	config := &FederationConfig{Port: ":8099", DBURL: ""}
	svc, err := NewFederationService(config, nil)
	require.NoError(t, err)
	assert.NotNil(t, svc)
	assert.NotNil(t, svc.logger)
}

func TestStartShutdown(t *testing.T) {
	config := &FederationConfig{Port: ":0", DBURL: ""}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := NewFederationService(config, logger)
	require.NoError(t, err)

	svc.server = &http.Server{
		Addr: ":0",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}
	err = svc.Shutdown(nil)
	assert.NoError(t, err)
}
