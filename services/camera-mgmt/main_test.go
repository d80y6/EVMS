package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	damv1 "github.com/dam-vms/dam/api/v1"
	"github.com/dam-vms/dam/pkg/common"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func setupTestDB(t *testing.T) *sqlx.DB {
	dbURL := os.Getenv("TEST_DB_URL")
	if dbURL == "" {
		t.Skip("TEST_DB_URL not set, skipping database tests")
	}
	db, err := sqlx.Connect("postgres", dbURL)
	require.NoError(t, err)
	return db
}

func TestService_mapCameraToProto(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	svc := &CameraService{logger: logger}

	now := time.Now()
	desc := "Description"
	subURL := "rtsp://localhost/sub"
	c := Camera{
		ID:            "cam-1",
		SiteID:        "site-1",
		Name:          "Test Camera",
		Description:   &desc,
		ConnectionURL: "rtsp://localhost/live",
		SubstreamURL:  &subURL,
		Status:        "online",
		CreatedAt:     now,
	}

	proto := svc.mapCameraToProto(c)

	assert.Equal(t, c.ID, proto.Id)
	assert.Equal(t, c.SiteID, proto.SiteId)
	assert.Equal(t, c.Name, proto.Name)
	assert.Equal(t, desc, proto.Description)
	assert.Equal(t, c.ConnectionURL, proto.ConnectionUrl)
	assert.Equal(t, subURL, proto.SubstreamUrl)
	assert.Equal(t, c.Status, proto.Status)
	assert.Equal(t, now.Unix(), proto.CreatedAt.AsTime().Unix())
}

func TestMapCameraToProto_NilPtrs(t *testing.T) {
	svc := &CameraService{logger: slog.New(slog.NewJSONHandler(os.Stdout, nil))}
	c := Camera{
		ID:            "cam-1",
		SiteID:        "site-1",
		Name:          "Test",
		ConnectionURL: "rtsp://localhost/live",
		CreatedAt:     time.Now(),
	}
	proto := svc.mapCameraToProto(c)
	assert.Equal(t, "", proto.Description)
	assert.Equal(t, "", proto.SubstreamUrl)
	assert.Equal(t, "offline", proto.Status)
	assert.Equal(t, "", proto.OnvifUsername)
}

func TestMapCameraToProto_Defaults(t *testing.T) {
	os.Setenv("ENCRYPTION_KEY", "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	defer os.Unsetenv("ENCRYPTION_KEY")

	svc := &CameraService{logger: slog.New(slog.NewJSONHandler(os.Stdout, nil))}
	c := Camera{
		ID:     "cam-2",
		SiteID: "site-2",
		Name:   "Test",
		ConnectionURL: "rtsp://localhost/live",
		CreatedAt:     time.Now(),
		PtzProtocol:   "ONVIF",
		RetentionDays: 14,
		PrerecordSeconds: 10,
		OnvifData:    `{"profile":"main"}`,
		OnvifUsername: strPtr("admin"),
		OnvifPassword: strPtr(common.MustEncrypt("real-password")),
		Config:       `{"key":"val"}`,
	}
	proto := svc.mapCameraToProto(c)
	assert.Equal(t, "ONVIF", proto.PtzProtocol)
	assert.Equal(t, int32(14), proto.RetentionDays)
	assert.Equal(t, int32(10), proto.PrerecordSeconds)
	assert.Equal(t, `{"profile":"main"}`, proto.OnvifData)
	assert.Equal(t, "real-password", proto.OnvifPassword)
	assert.Equal(t, `{"key":"val"}`, proto.Config)
}

func strPtr(s string) *string { return &s }

func TestSQLNullString(t *testing.T) {
	assert.Equal(t, nil, sqlNullString(""))
	assert.Equal(t, "hello", sqlNullString("hello"))
}

func TestCheckCameraReachable_Listener(t *testing.T) {
	svc := &CameraService{}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	addr := ln.Addr().String()
	reachable := svc.checkCameraReachable("rtsp://" + addr)
	assert.True(t, reachable)
}

func TestCheckCameraReachable_Unreachable(t *testing.T) {
	svc := &CameraService{}
	reachable := svc.checkCameraReachable("rtsp://127.0.0.1:1")
	assert.False(t, reachable)
}

func TestCheckCameraReachable_NonRTSP(t *testing.T) {
	svc := &CameraService{}
	reachable := svc.checkCameraReachable("192.0.2.1")
	assert.False(t, reachable)
}

func TestCheckCameraReachable_WithCredentials(t *testing.T) {
	svc := &CameraService{}
	reachable := svc.checkCameraReachable("rtsp://admin:pass@127.0.0.1:1")
	assert.False(t, reachable)
}

func TestCheckCameraReachable_CustomPort(t *testing.T) {
	svc := &CameraService{}
	reachable := svc.checkCameraReachable("rtsp://127.0.0.1:9999")
	assert.False(t, reachable)
}

func TestDefaultCameraConfig(t *testing.T) {
	cfg := DefaultCameraConfig()
	assert.Equal(t, ":50051", cfg.GRPCPort)
	assert.Equal(t, "30s", cfg.GracefulTimeout.String())
}

func TestDefaultCameraConfig_EnvOverride(t *testing.T) {
	os.Setenv("GRPC_PORT", ":50052")
	os.Setenv("DB_URL", "postgres://localhost:5432/test")
	defer func() {
		os.Unsetenv("GRPC_PORT")
		os.Unsetenv("DB_URL")
	}()
	cfg := DefaultCameraConfig()
	assert.Equal(t, ":50052", cfg.GRPCPort)
	assert.Equal(t, "postgres://localhost:5432/test", cfg.DBURL)
}

func TestTenantServerInterceptor(t *testing.T) {
	t.Run("sets tenant from metadata", func(t *testing.T) {
		md := metadata.Pairs("tenant_id", "tenant-1")
		ctx := metadata.NewIncomingContext(context.Background(), md)
		var capturedCtx context.Context
		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			capturedCtx = ctx
			return "ok", nil
		}
		_, err := tenantServerInterceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
		require.NoError(t, err)
		assert.Equal(t, "tenant-1", common.TenantFromContext(capturedCtx))
	})

	t.Run("no tenant metadata", func(t *testing.T) {
		ctx := context.Background()
		var capturedCtx context.Context
		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			capturedCtx = ctx
			return "ok", nil
		}
		_, err := tenantServerInterceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
		require.NoError(t, err)
		assert.Equal(t, "", common.TenantFromContext(capturedCtx))
	})

	t.Run("empty tenant value", func(t *testing.T) {
		md := metadata.Pairs("tenant_id", "")
		ctx := metadata.NewIncomingContext(context.Background(), md)
		var capturedCtx context.Context
		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			capturedCtx = ctx
			return "ok", nil
		}
		_, err := tenantServerInterceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
		require.NoError(t, err)
		assert.Equal(t, "", common.TenantFromContext(capturedCtx))
	})
}

func TestShutdown_NilServer(t *testing.T) {
	svc := &CameraService{}
	err := svc.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestShutdown_NilMembers(t *testing.T) {
	svc := &CameraService{}
	err := svc.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestStart_NilConfig(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	_, err := NewCameraService(&CameraConfig{}, logger)
	assert.Error(t, err)
}

func TestNewCameraService_NoDBURL(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	_, err := NewCameraService(&CameraConfig{DBURL: ""}, logger)
	assert.ErrorContains(t, err, "database URL is required")
}

func TestValidateCamera(t *testing.T) {
	tests := []struct {
		name    string
		req     *damv1.CreateCameraRequest
		wantErr bool
	}{
		{"valid minimal", &damv1.CreateCameraRequest{SiteId: "s1", Name: "cam", ConnectionUrl: "rtsp://example/stream"}, false},
		{"valid full", &damv1.CreateCameraRequest{
			SiteId: "s1", Name: "cam", ConnectionUrl: "rtsp://example/stream",
			SubstreamUrl: "rtsp://example/sub", PtzProtocol: "onvif",
			RetentionDays: 30, PrerecordSeconds: 5,
			OnvifUsername: "admin", OnvifPassword: "pass",
		}, false},
		{"empty name", &damv1.CreateCameraRequest{SiteId: "s1", ConnectionUrl: "rtsp://example/stream"}, true},
		{"name too long", &damv1.CreateCameraRequest{SiteId: "s1", Name: strings.Repeat("a", 256), ConnectionUrl: "rtsp://example/stream"}, true},
		{"empty site_id", &damv1.CreateCameraRequest{Name: "cam", ConnectionUrl: "rtsp://example/stream"}, true},
		{"empty connection_url", &damv1.CreateCameraRequest{SiteId: "s1", Name: "cam"}, true},
		{"invalid connection_url", &damv1.CreateCameraRequest{SiteId: "s1", Name: "cam", ConnectionUrl: "ftp://bad"}, true},
		{"invalid substream_url", &damv1.CreateCameraRequest{SiteId: "s1", Name: "cam", ConnectionUrl: "rtsp://good", SubstreamUrl: "ftp://bad"}, true},
		{"invalid ptz_protocol", &damv1.CreateCameraRequest{SiteId: "s1", Name: "cam", ConnectionUrl: "rtsp://example", PtzProtocol: "invalid"}, true},
		{"retention_days too high", &damv1.CreateCameraRequest{SiteId: "s1", Name: "cam", ConnectionUrl: "rtsp://example", RetentionDays: 9999}, true},
		{"prerecord_seconds too high", &damv1.CreateCameraRequest{SiteId: "s1", Name: "cam", ConnectionUrl: "rtsp://example", PrerecordSeconds: 60}, true},
		{"onvif username without password", &damv1.CreateCameraRequest{SiteId: "s1", Name: "cam", ConnectionUrl: "rtsp://example", OnvifUsername: "admin"}, true},
		{"onvif password without username", &damv1.CreateCameraRequest{SiteId: "s1", Name: "cam", ConnectionUrl: "rtsp://example", OnvifPassword: "pass"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCamera(tt.req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
