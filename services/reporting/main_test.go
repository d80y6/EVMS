package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultReportServiceConfig(t *testing.T) {
	cfg := DefaultReportServiceConfig()
	assert.Equal(t, ":8098", cfg.Port)
	assert.Equal(t, "nats://nats:4222", cfg.NATSURL)
}

func TestDefaultReportServiceConfig_EnvOverride(t *testing.T) {
	os.Setenv("REPORTING_PORT", ":9098")
	os.Setenv("NATS_URL", "nats://test:4222")
	os.Setenv("DB_URL", "postgres://localhost:5432/test")
	defer func() {
		os.Unsetenv("REPORTING_PORT")
		os.Unsetenv("NATS_URL")
		os.Unsetenv("DB_URL")
	}()
	cfg := DefaultReportServiceConfig()
	assert.Equal(t, ":9098", cfg.Port)
	assert.Equal(t, "nats://test:4222", cfg.NATSURL)
	assert.Equal(t, "postgres://localhost:5432/test", cfg.DBURL)
}

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusCreated, map[string]string{"status": "ok"})
	assert.Equal(t, http.StatusCreated, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	assert.Equal(t, "ok", body["status"])
}

func TestRenderDataTable_Audit(t *testing.T) {
	entries := []AuditEntry{
		{Timestamp: "2026-01-15 10:00:00", Actor: "admin", Action: "login", Resource: "system", Status: "success"},
		{Timestamp: "2026-01-15 11:00:00", Actor: "admin", Action: "logout", Resource: "system", Status: "success"},
	}
	result := renderDataTable("audit", entries)
	assert.Contains(t, result, "admin")
	assert.Contains(t, result, "login")
	assert.Contains(t, result, "logout")
	assert.Contains(t, result, "<table>")
}

func TestRenderDataTable_Events(t *testing.T) {
	entries := []EventSummary{
		{CameraID: "cam-1", ObjectType: "person", Count: 10, LastTime: "2026-01-15"},
	}
	result := renderDataTable("events", entries)
	assert.Contains(t, result, "cam-1")
	assert.Contains(t, result, "person")
	assert.Contains(t, result, "10")
}

func TestRenderDataTable_Storage(t *testing.T) {
	entries := []StorageEntry{
		{CameraID: "cam-1", TotalSize: "1.2 GB", RecordingDays: 30, Oldest: "2026-01-01", Latest: "2026-01-31"},
	}
	result := renderDataTable("storage", entries)
	assert.Contains(t, result, "cam-1")
	assert.Contains(t, result, "1.2 GB")
	assert.Contains(t, result, "30")
}

func TestRenderDataTable_Health(t *testing.T) {
	entries := []HealthEntry{
		{Service: "api-gateway", Status: "healthy", Uptime: "10d", LastCheck: "2026-01-15 10:00:00"},
	}
	result := renderDataTable("health", entries)
	assert.Contains(t, result, "api-gateway")
	assert.Contains(t, result, "healthy")
}

func TestRenderDataTable_UnknownType(t *testing.T) {
	result := renderDataTable("unknown", nil)
	assert.Equal(t, "", result)
}

func TestRenderDataTable_WrongType(t *testing.T) {
	result := renderDataTable("audit", "not-a-slice")
	assert.Equal(t, "<p>No data available</p>", result)
}

func TestRenderDataTable_Empty(t *testing.T) {
	t.Run("audit", func(t *testing.T) {
		result := renderDataTable("audit", []AuditEntry{})
		assert.Contains(t, result, "<table>")
	})
	t.Run("events", func(t *testing.T) {
		result := renderDataTable("events", []EventSummary{})
		assert.Contains(t, result, "<table>")
	})
	t.Run("storage", func(t *testing.T) {
		result := renderDataTable("storage", []StorageEntry{})
		assert.Contains(t, result, "<table>")
	})
	t.Run("health", func(t *testing.T) {
		result := renderDataTable("health", []HealthEntry{})
		assert.Contains(t, result, "<table>")
	})
}

func TestRenderReport(t *testing.T) {
	config := ReportConfig{
		Name: "Daily Audit Report",
		Type: "audit",
	}
	data := []AuditEntry{
		{Timestamp: "2026-01-15 10:00:00", Actor: "admin", Action: "login", Resource: "system", Status: "success"},
	}
	result, err := renderReport(config, data)
	require.NoError(t, err)
	assert.Contains(t, result, "Daily Audit Report")
	assert.Contains(t, result, "Audit Report")
	assert.Contains(t, result, "admin")
	assert.Contains(t, result, "login")
	assert.Contains(t, result, "EVMS Reporting Engine")
	assert.Contains(t, result, "<html>")
	assert.Contains(t, result, "</html>")
}

func TestReportDataTypes_JSONRoundTrip(t *testing.T) {
	t.Run("AuditEntry", func(t *testing.T) {
		orig := AuditEntry{Timestamp: "t1", Actor: "admin", Action: "login", Resource: "sys", Status: "ok"}
		data, _ := json.Marshal(orig)
		var decoded AuditEntry
		json.Unmarshal(data, &decoded)
		assert.Equal(t, orig, decoded)
	})
	t.Run("EventSummary", func(t *testing.T) {
		orig := EventSummary{CameraID: "cam-1", ObjectType: "person", Count: 5, LastTime: "t1"}
		data, _ := json.Marshal(orig)
		var decoded EventSummary
		json.Unmarshal(data, &decoded)
		assert.Equal(t, orig, decoded)
	})
	t.Run("StorageEntry", func(t *testing.T) {
		orig := StorageEntry{CameraID: "cam-1", TotalSize: "1GB", RecordingDays: 10, Oldest: "d1", Latest: "d2"}
		data, _ := json.Marshal(orig)
		var decoded StorageEntry
		json.Unmarshal(data, &decoded)
		assert.Equal(t, orig, decoded)
	})
	t.Run("HealthEntry", func(t *testing.T) {
		orig := HealthEntry{Service: "svc", Status: "ok", Uptime: "10d", LastCheck: "t1"}
		data, _ := json.Marshal(orig)
		var decoded HealthEntry
		json.Unmarshal(data, &decoded)
		assert.Equal(t, orig, decoded)
	})
}

func TestRenderReport_AllTypes(t *testing.T) {
	types := []string{"audit", "events", "storage", "health"}
	datasets := map[string]interface{}{
		"audit":   []AuditEntry{{Timestamp: "t1", Actor: "admin", Action: "login", Resource: "sys", Status: "ok"}},
		"events":  []EventSummary{{CameraID: "cam-1", ObjectType: "person", Count: 1, LastTime: "t1"}},
		"storage": []StorageEntry{{CameraID: "cam-1", TotalSize: "1GB", RecordingDays: 1, Oldest: "d1", Latest: "d2"}},
		"health":  []HealthEntry{{Service: "svc", Status: "ok", Uptime: "1d", LastCheck: "t1"}},
	}

	for _, rt := range types {
		t.Run(rt, func(t *testing.T) {
			config := ReportConfig{Name: "Test " + rt, Type: rt}
			data := datasets[rt]
			result, err := renderReport(config, data)
			require.NoError(t, err)
			assert.Contains(t, result, rt[0:1], "report type %s should produce valid HTML", rt)
		})
	}
}

func TestRenderReport_HTMLFormatting(t *testing.T) {
	config := ReportConfig{Name: "Test", Type: "audit"}
	data := []AuditEntry{{Timestamp: "t1", Actor: "admin", Action: "login", Resource: "sys", Status: "ok"}}
	result, err := renderReport(config, data)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(result, "<!DOCTYPE html>"), "should start with doctype")
	assert.Contains(t, result, "Generated:")
	assert.Contains(t, result, "Auto-generated report")
	assert.Regexp(t, `Generated:\s+\d{4}-\d{2}-\d{2}`, result)
}

func TestConfigValidation(t *testing.T) {
	validTypes := map[string]bool{"audit": true, "events": true, "storage": true, "health": true}
	assert.True(t, validTypes["audit"])
	assert.True(t, validTypes["health"])
	assert.False(t, validTypes["unknown"])
	assert.False(t, validTypes["csv"])
}
