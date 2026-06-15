package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/dam-vms/dam/pkg/onvif"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupJWT(t *testing.T) string {
	t.Helper()
	os.Setenv("JWT_SECRET", "test-secret-for-onvif-events-test")
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

func newTestOnvifEventsService() *OnvifEventsService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &OnvifEventsService{
		config: &OnvifEventsConfig{
			Port:    ":8092",
			NATSURL: "nats://localhost:4222",
			DBURL:   "",
		},
		logger: logger,
		subs:   make(map[string]*subscription),
	}
}

func authenticatedRequest(method, path string, body []byte, token string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func TestDefaultOnvifEventsConfig(t *testing.T) {
	cfg := DefaultOnvifEventsConfig()
	assert.Equal(t, ":8092", cfg.Port)
	assert.Equal(t, "nats://nats:4222", cfg.NATSURL)
	assert.Equal(t, "", cfg.DBURL)
}

func TestDefaultOnvifEventsConfig_EnvOverride(t *testing.T) {
	os.Setenv("ONVIF_EVENTS_PORT", ":9092")
	os.Setenv("NATS_URL", "nats://test:4222")
	os.Setenv("DB_URL", "postgres://localhost:5432/test")
	defer func() {
		os.Unsetenv("ONVIF_EVENTS_PORT")
		os.Unsetenv("NATS_URL")
		os.Unsetenv("DB_URL")
	}()
	cfg := DefaultOnvifEventsConfig()
	assert.Equal(t, ":9092", cfg.Port)
	assert.Equal(t, "nats://test:4222", cfg.NATSURL)
	assert.Equal(t, "postgres://localhost:5432/test", cfg.DBURL)
}

func TestHandleSubscribe_MethodNotAllowed(t *testing.T) {
	s := newTestOnvifEventsService()
	wrapped := common.JWTAuthMiddleware(s.handleSubscribe)
	token := setupJWT(t)

	rr := httptest.NewRecorder()
	req := authenticatedRequest(http.MethodGet, "/onvif-events/subscribe", nil, token)
	wrapped(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleSubscribe_InvalidJSON(t *testing.T) {
	s := newTestOnvifEventsService()
	wrapped := common.JWTAuthMiddleware(s.handleSubscribe)
	token := setupJWT(t)

	rr := httptest.NewRecorder()
	req := authenticatedRequest(http.MethodPost, "/onvif-events/subscribe", []byte(`not-json`), token)
	wrapped(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleSubscribe_MissingFields(t *testing.T) {
	s := newTestOnvifEventsService()
	wrapped := common.JWTAuthMiddleware(s.handleSubscribe)
	token := setupJWT(t)

	tests := []struct {
		name string
		body string
	}{
		{"empty", `{}`},
		{"missing camera_id", `{"onvif_device_url":"http://camera"}`},
		{"missing device_url", `{"camera_id":"cam-1"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := authenticatedRequest(http.MethodPost, "/onvif-events/subscribe", []byte(tt.body), token)
			wrapped(rr, req)
			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

func TestHandleSubscribe_DuplicateCamera(t *testing.T) {
	s := newTestOnvifEventsService()
	s.subs["cam-1"] = &subscription{CameraID: "cam-1"}
	wrapped := common.JWTAuthMiddleware(s.handleSubscribe)
	token := setupJWT(t)

	rr := httptest.NewRecorder()
	req := authenticatedRequest(http.MethodPost, "/onvif-events/subscribe", []byte(`{"camera_id":"cam-1","onvif_device_url":"http://camera"}`), token)
	wrapped(rr, req)
	assert.Equal(t, http.StatusConflict, rr.Code)
}

func TestHandleSubscribe_NoAuth(t *testing.T) {
	s := newTestOnvifEventsService()
	wrapped := common.JWTAuthMiddleware(s.handleSubscribe)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/onvif-events/subscribe", bytes.NewReader([]byte(`{}`)))
	wrapped(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleUnsubscribe_MethodNotAllowed(t *testing.T) {
	s := newTestOnvifEventsService()
	wrapped := common.JWTAuthMiddleware(s.handleUnsubscribe)
	token := setupJWT(t)

	rr := httptest.NewRecorder()
	req := authenticatedRequest(http.MethodPost, "/onvif-events/subscribe/", nil, token)
	wrapped(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleUnsubscribe_MissingCameraID(t *testing.T) {
	s := newTestOnvifEventsService()
	wrapped := common.JWTAuthMiddleware(s.handleUnsubscribe)
	token := setupJWT(t)

	rr := httptest.NewRecorder()
	req := authenticatedRequest(http.MethodDelete, "/onvif-events/subscribe/", nil, token)
	wrapped(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleUnsubscribe_NotFound(t *testing.T) {
	s := newTestOnvifEventsService()
	wrapped := common.JWTAuthMiddleware(s.handleUnsubscribe)
	token := setupJWT(t)

	rr := httptest.NewRecorder()
	req := authenticatedRequest(http.MethodDelete, "/onvif-events/subscribe/nonexistent", nil, token)
	wrapped(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandleUnsubscribe_Success(t *testing.T) {
	s := newTestOnvifEventsService()
	s.subs["cam-1"] = &subscription{
		CameraID: "cam-1",
		stopCh:   make(chan struct{}),
	}
	wrapped := common.JWTAuthMiddleware(s.handleUnsubscribe)
	token := setupJWT(t)

	rr := httptest.NewRecorder()
	req := authenticatedRequest(http.MethodDelete, "/onvif-events/subscribe/cam-1", nil, token)
	wrapped(rr, req)
	assert.Equal(t, http.StatusNoContent, rr.Code)
	_, exists := s.subs["cam-1"]
	assert.False(t, exists, "subscription should be removed")
}

func TestHandleListSubscriptions_MethodNotAllowed(t *testing.T) {
	s := newTestOnvifEventsService()
	wrapped := common.JWTAuthMiddleware(s.handleListSubscriptions)
	token := setupJWT(t)

	rr := httptest.NewRecorder()
	req := authenticatedRequest(http.MethodPost, "/onvif-events/subscriptions", nil, token)
	wrapped(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleListSubscriptions_Empty(t *testing.T) {
	s := newTestOnvifEventsService()
	wrapped := common.JWTAuthMiddleware(s.handleListSubscriptions)
	token := setupJWT(t)

	rr := httptest.NewRecorder()
	req := authenticatedRequest(http.MethodGet, "/onvif-events/subscriptions", nil, token)
	wrapped(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var subs []subscriptionInfo
	err := json.Unmarshal(rr.Body.Bytes(), &subs)
	require.NoError(t, err)
	assert.Len(t, subs, 0)
}

func TestHandleListSubscriptions_WithEntries(t *testing.T) {
	s := newTestOnvifEventsService()
	s.subs["cam-1"] = &subscription{CameraID: "cam-1", DeviceURL: "http://cam1", PullPointURL: "http://pull/cam1"}
	s.subs["cam-2"] = &subscription{CameraID: "cam-2", DeviceURL: "http://cam2", PullPointURL: "http://pull/cam2"}
	wrapped := common.JWTAuthMiddleware(s.handleListSubscriptions)
	token := setupJWT(t)

	rr := httptest.NewRecorder()
	req := authenticatedRequest(http.MethodGet, "/onvif-events/subscriptions", nil, token)
	wrapped(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var subs []subscriptionInfo
	err := json.Unmarshal(rr.Body.Bytes(), &subs)
	require.NoError(t, err)
	require.Len(t, subs, 2)

	camIDs := make(map[string]bool)
	for _, sub := range subs {
		camIDs[sub.CameraID] = true
	}
	assert.True(t, camIDs["cam-1"])
	assert.True(t, camIDs["cam-2"])
}

func TestInsertEvent_NilDB(t *testing.T) {
	s := newTestOnvifEventsService()
	sub := &subscription{CameraID: "cam-1"}
	err := s.insertEvent(nil, sub, onvif.ONVIFEvent{
		Topic: "test",
		Data:  map[string]interface{}{},
	}, "test-event")
	assert.NoError(t, err)
}

func TestClose_NilDBAndNATS(t *testing.T) {
	s := newTestOnvifEventsService()
	// Add a subscription to ensure Close cleans up
	s.subs["cam-1"] = &subscription{
		CameraID: "cam-1",
		stopCh:   make(chan struct{}),
	}
	err := s.Close()
	assert.NoError(t, err)
	assert.Nil(t, s.subs)
}

func TestClose_EmptySubs(t *testing.T) {
	s := newTestOnvifEventsService()
	err := s.Close()
	assert.NoError(t, err)
}

func TestSubscribeUnsubscribe_Concurrent(t *testing.T) {
	s := newTestOnvifEventsService()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			s.mu.Lock()
			s.subs[fmt.Sprintf("cam-%d", id)] = &subscription{CameraID: fmt.Sprintf("cam-%d", id)}
			s.mu.Unlock()
		}(i)
	}
	wg.Wait()

	s.mu.Lock()
	assert.Len(t, s.subs, 10)
	s.mu.Unlock()
}

func TestHandleSubscribe_WithBadDeviceURL(t *testing.T) {
	s := newTestOnvifEventsService()
	wrapped := common.JWTAuthMiddleware(s.handleSubscribe)
	token := setupJWT(t)

	rr := httptest.NewRecorder()
	req := authenticatedRequest(http.MethodPost, "/onvif-events/subscribe",
		[]byte(`{"camera_id":"cam-1","onvif_device_url":"http://nonexistent:9999"}`), token)
	wrapped(rr, req)
	assert.Equal(t, http.StatusBadGateway, rr.Code)
}

func TestHandleListSubscriptions_NoAuth(t *testing.T) {
	s := newTestOnvifEventsService()
	wrapped := common.JWTAuthMiddleware(s.handleListSubscriptions)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/onvif-events/subscriptions", nil)
	wrapped(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleUnsubscribe_NoAuth(t *testing.T) {
	s := newTestOnvifEventsService()
	wrapped := common.JWTAuthMiddleware(s.handleUnsubscribe)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/onvif-events/subscribe/cam-1", nil)
	wrapped(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}
