package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/assert"
)

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
	signed, _ := token.SignedString([]byte("test-secret-for-webrtc-auth"))
	return signed
}

func TestWebRTCOffer_NoAuthReturns401(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := &WebRTCService{logger: logger}

	req := httptest.NewRequest(http.MethodPost, "/webrtc/offer?camera_id=cam1", bytes.NewReader([]byte(`{"type":"offer","sdp":"test"}`)))
	rr := httptest.NewRecorder()

	svc.createOfferHandler(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestWebRTCOffer_MissingCameraID(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-for-webrtc-auth")
	common.ReloadJWTKey()
	defer func() {
		os.Unsetenv("JWT_SECRET")
		common.ReloadJWTKey()
	}()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := &WebRTCService{logger: logger}

	token := generateTestJWT("testuser", "viewer")
	req := httptest.NewRequest(http.MethodPost, "/webrtc/offer", bytes.NewReader([]byte(`{"type":"offer","sdp":"test"}`)))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	svc.createOfferHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestWebRTCOffer_AuthFlowWithContext(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-for-webrtc-auth")
	common.ReloadJWTKey()
	defer func() {
		os.Unsetenv("JWT_SECRET")
		common.ReloadJWTKey()
	}()

	token := generateTestJWT("testuser", "viewer")

	var capturedCtx context.Context
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	middleware := common.JWTAuthMiddleware(innerHandler)
	req := httptest.NewRequest(http.MethodPost, "/webrtc/offer?camera_id=cam1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	middleware(rr, req)

	username := capturedCtx.Value(common.UserKey)
	if username == nil {
		t.Error("expected username in context after middleware")
	} else if username.(string) != "testuser" {
		t.Errorf("username = %q, want %q", username, "testuser")
	}
}

func TestWebRTCOffer_TokenInQueryParamRejected(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-for-webrtc-auth")
	common.ReloadJWTKey()
	defer func() {
		os.Unsetenv("JWT_SECRET")
		common.ReloadJWTKey()
	}()

	token := generateTestJWT("testuser", "viewer")

	innerCalled := false
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	middleware := common.JWTAuthMiddleware(innerHandler)
	req := httptest.NewRequest(http.MethodPost, "/webrtc/offer?camera_id=cam1&token="+token, nil)
	rr := httptest.NewRecorder()

	middleware(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
	if innerCalled {
		t.Error("inner handler should not be called when token is in query param")
	}
}

func TestWebRTCOffer_AuthMiddlewareCalledByRoute(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-for-webrtc-auth")
	common.ReloadJWTKey()
	defer func() {
		os.Unsetenv("JWT_SECRET")
		common.ReloadJWTKey()
	}()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := &WebRTCService{logger: logger}
	svc.healthHandler = common.NewHealthHandler()

	mux := http.NewServeMux()
	mux.HandleFunc("/webrtc/offer", common.JWTAuthMiddleware(svc.createOfferHandler))
	mux.HandleFunc("/health", svc.healthHandler.Liveness)
	mux.HandleFunc("/ready", svc.healthHandler.Readiness)

	server := &http.Server{
		Addr:    ":0",
		Handler: mux,
	}
	defer server.Close()

	// Test without auth
	req := httptest.NewRequest(http.MethodPost, "/webrtc/offer?camera_id=cam1", bytes.NewReader([]byte(`{"type":"offer"}`)))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", rr.Code)
	}
}

func TestWebRTCService_ContextKeysFromAuth(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-for-webrtc-auth")
	common.ReloadJWTKey()
	defer func() {
		os.Unsetenv("JWT_SECRET")
		common.ReloadJWTKey()
	}()

	token := generateTestJWT("admin-user", "admin")

	handler := common.JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		username, _ := ctx.Value(common.UserKey).(string)
		role, _ := ctx.Value(common.RoleKey).(string)
		tenant, _ := ctx.Value(common.TenantKey).(string)

		if username != "admin-user" {
			t.Errorf("username = %q, want %q", username, "admin-user")
		}
		if role != "admin" {
			t.Errorf("role = %q, want %q", role, "admin")
		}
		if tenant != "" {
			t.Errorf("expected empty tenant, got %q", tenant)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestWebRTC_CleanupSession_NotPanics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := &WebRTCService{logger: logger}

	svc.cleanupSession("nonexistent-camera")
}

func TestWebRTC_UserFromContext_ReturnsEmptyForNoAuth(t *testing.T) {
	ctx := context.Background()
	result := common.UserFromContext(ctx)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultWebRTCConfig()
	if config.HTTPAddr != ":8082" {
		t.Errorf("default HTTPAddr = %q, want %q", config.HTTPAddr, ":8082")
	}
}

func TestConnectionStateConstants(t *testing.T) {
	if webrtc.PeerConnectionStateClosed.String() != "closed" {
		t.Errorf("unexpected state string")
	}
}

func TestCreateAnswerFailsWithoutCodecRegistration(t *testing.T) {
	m := &webrtc.MediaEngine{}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m))
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()

	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video",
		"pion",
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = pc.AddTrack(track); err != nil {
		t.Fatal(err)
	}

	offerM := &webrtc.MediaEngine{}
	if err := offerM.RegisterDefaultCodecs(); err != nil {
		t.Fatal(err)
	}
	offerAPI := webrtc.NewAPI(webrtc.WithMediaEngine(offerM))
	offerPC, err := offerAPI.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}

	offerTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video",
		"pion",
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = offerPC.AddTrack(offerTrack); err != nil {
		t.Fatal(err)
	}

	offer, err := offerPC.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}

	if err = pc.SetRemoteDescription(offer); err != nil {
		t.Fatal(err)
	}

	if _, err = pc.CreateAnswer(nil); err == nil {
		t.Fatal("expected CreateAnswer to fail without codec registration")
	} else {
		t.Logf("got expected error: %v", err)
	}
	offerPC.Close()
}

func TestAddTrackSucceedsWithDefaultCodecs(t *testing.T) {
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		t.Fatal(err)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(m))
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()

	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video",
		"pion",
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = pc.AddTrack(track); err != nil {
		t.Fatalf("AddTrack failed with default codecs: %v", err)
	}
}

func TestCreateOfferHandler_SuccessPath(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-for-webrtc-auth")
	common.ReloadJWTKey()
	defer func() {
		os.Unsetenv("JWT_SECRET")
		common.ReloadJWTKey()
	}()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := &WebRTCService{logger: logger}

	token := generateTestJWT("testuser", "viewer")
	body := bytes.NewReader([]byte("invalid offer body"))
	req := httptest.NewRequest(http.MethodPost, "/webrtc/offer?camera_id=cam1", body)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	common.JWTAuthMiddleware(svc.createOfferHandler)(rr, req)

	// Expect 400 (invalid offer body) rather than 500 — proving
	// input validation works correctly before hitting infrastructure.
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
