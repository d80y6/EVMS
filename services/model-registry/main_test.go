package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupJWT(t *testing.T) string {
	t.Helper()
	os.Setenv("JWT_SECRET", "test-secret-model-registry")
	common.ReloadJWTKey()
	t.Cleanup(func() {
		os.Unsetenv("JWT_SECRET")
		common.ReloadJWTKey()
	})
	token, err := common.SignJWT(&common.Claims{
		Username: "test-user",
		Role:     "admin",
	})
	require.NoError(t, err)
	return token
}

func authRequest(method, path string, body []byte, token string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func TestDefaultModelConfig(t *testing.T) {
	cfg := DefaultModelConfig()
	assert.Equal(t, ":8098", cfg.Port)
	assert.Equal(t, "nats://nats:4222", cfg.NATSURL)
}

func TestDefaultModelConfig_EnvOverride(t *testing.T) {
	os.Setenv("MODEL_REGISTRY_PORT", ":9098")
	os.Setenv("NATS_URL", "nats://test:4222")
	os.Setenv("DB_URL", "postgres://localhost:5432/test")
	defer func() {
		os.Unsetenv("MODEL_REGISTRY_PORT")
		os.Unsetenv("NATS_URL")
		os.Unsetenv("DB_URL")
	}()
	cfg := DefaultModelConfig()
	assert.Equal(t, ":9098", cfg.Port)
	assert.Equal(t, "nats://test:4222", cfg.NATSURL)
	assert.Equal(t, "postgres://localhost:5432/test", cfg.DBURL)
}

func TestWriteJSON_ModelRegistry(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusCreated, map[string]string{"status": "created"})
	assert.Equal(t, http.StatusCreated, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	assert.Equal(t, "created", body["status"])
}

func TestCreateModel_DBNil(t *testing.T) {
	rr := httptest.NewRecorder()
	createModel(nil, rr, httptest.NewRequest(http.MethodPost, "/api/models", bytes.NewReader([]byte(`{"name":"test"}`))))
	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	assert.Equal(t, "database not configured", body["error"])
}

func TestListModels_DBNil(t *testing.T) {
	rr := httptest.NewRecorder()
	listModels(nil, rr, httptest.NewRequest(http.MethodGet, "/api/models", nil))
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestGetModel_DBNil(t *testing.T) {
	rr := httptest.NewRecorder()
	getModel(nil, rr, httptest.NewRequest(http.MethodGet, "/api/models/", nil), "test-id")
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestActivateVersion_DBNil(t *testing.T) {
	rr := httptest.NewRecorder()
	activateVersion(nil, rr, httptest.NewRequest(http.MethodPost, "/api/models/test/activate", nil), "test-id")
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestDeployCanary_DBNil(t *testing.T) {
	rr := httptest.NewRecorder()
	deployCanary(nil, rr, httptest.NewRequest(http.MethodPost, "/api/models/test/canary", bytes.NewReader([]byte(`{"percent":10}`))), "test-id")
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestPromoteCanary_DBNil(t *testing.T) {
	rr := httptest.NewRecorder()
	promoteCanary(nil, nil, rr, httptest.NewRequest(http.MethodPost, "/api/models/test/promote", nil), "test-id")
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestRollback_DBNil(t *testing.T) {
	rr := httptest.NewRecorder()
	rollback(nil, nil, rr, httptest.NewRequest(http.MethodPost, "/api/models/test/rollback", nil), "test-id")
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestModelRegistry_AuthMiddleware(t *testing.T) {
	testHandler := common.JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("no auth header", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
		testHandler(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("invalid token", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := authRequest(http.MethodGet, "/api/models", nil, "bad-token")
		testHandler(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("valid token", func(t *testing.T) {
		token := setupJWT(t)
		rr := httptest.NewRecorder()
		req := authRequest(http.MethodGet, "/api/models", nil, token)
		testHandler(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}

func TestModelRouting_MethodNotAllowed(t *testing.T) {
	token := setupJWT(t)
	handler := common.JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listModels(nil, w, r)
		case http.MethodPost:
			createModel(nil, w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	t.Run("delete not allowed", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := authRequest(http.MethodDelete, "/api/models", nil, token)
		handler(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestClose_NilMembers(t *testing.T) {
	r := &ModelRegistry{}
	err := r.Close()
	assert.NoError(t, err)
}

func TestClose_WithServer(t *testing.T) {
	r := &ModelRegistry{
		httpSrv: &http.Server{},
	}
	err := r.Close()
	assert.NoError(t, err)
}

func TestModelJSON_RoundTrip(t *testing.T) {
	orig := Model{
		ID:        "m-1",
		Name:      "yolov8",
		Version:   3,
		Status:    "active",
		ModelPath: "/models/yolov8/v3",
		Metrics:   json.RawMessage(`{"accuracy":0.95}`),
	}
	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded Model
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, orig.ID, decoded.ID)
	assert.Equal(t, orig.Name, decoded.Name)
	assert.Equal(t, orig.Version, decoded.Version)
	assert.Equal(t, orig.Status, decoded.Status)
}
