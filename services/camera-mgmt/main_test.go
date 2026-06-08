package main

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NOTE: These tests require a running database if not using mocks.
// For the sake of this enterprise-grade foundation, we will assume
// the environment provides a test database via DB_URL.
// If not, we skip the integration-heavy parts.

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
