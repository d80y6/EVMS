package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAuditService() *AuditService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &AuditService{
		logger: logger,
		port:   ":8093",
	}
}

func TestComputeHash_Deterministic(t *testing.T) {
	s := newTestAuditService()
	h1 := s.computeHash("prev123", "2026-01-01T00:00:00Z", "login", "admin", "user", "u-1", "details")
	h2 := s.computeHash("prev123", "2026-01-01T00:00:00Z", "login", "admin", "user", "u-1", "details")
	assert.Equal(t, h1, h2, "hash should be deterministic")
}

func TestComputeHash_DifferentInputsDiffer(t *testing.T) {
	s := newTestAuditService()
	h1 := s.computeHash("prev", "ts1", "act1", "actor1", "res1", "rid1", "det1")
	h2 := s.computeHash("prev", "ts1", "act2", "actor1", "res1", "rid1", "det1")
	assert.NotEqual(t, h1, h2, "different actions should produce different hashes")
}

func TestComputeHash_Format(t *testing.T) {
	s := newTestAuditService()
	h := s.computeHash("prev", "ts", "act", "actor", "res", "rid", "det")
	assert.Len(t, h, 64, "SHA256 hex should be 64 characters")
}

func TestLogEntry_FirstEntryUsesZeroHash(t *testing.T) {
	s := newTestAuditService()
	s.logEntry(&AuditEntry{
		ID: "e1", Timestamp: "2026-01-01T00:00:00Z",
		Action: "login", Actor: "admin", ResourceType: "user", ResourceID: "u-1",
	})
	require.Len(t, s.entries, 1)
	assert.Equal(t, "0000000000000000000000000000000000000000000000000000000000000000", s.entries[0].PreviousHash)
	assert.NotEqual(t, "", s.entries[0].Hash)
}

func TestLogEntry_ChainLinking(t *testing.T) {
	s := newTestAuditService()
	s.logEntry(&AuditEntry{ID: "e1", Timestamp: "t1", Action: "a1", Actor: "u1", ResourceType: "r", ResourceID: "1"})
	s.logEntry(&AuditEntry{ID: "e2", Timestamp: "t2", Action: "a2", Actor: "u1", ResourceType: "r", ResourceID: "2"})
	require.Len(t, s.entries, 2)
	assert.Equal(t, s.entries[0].Hash, s.entries[1].PreviousHash, "second entry should reference first entry's hash")
}

func TestLogEntry_Dedup(t *testing.T) {
	s := newTestAuditService()
	s.logEntry(&AuditEntry{ID: "e1", Timestamp: "t1", Action: "a1", Actor: "u1", ResourceType: "r", ResourceID: "1"})
	s.logEntry(&AuditEntry{ID: "e1", Timestamp: "t2", Action: "a2", Actor: "u2", ResourceType: "r", ResourceID: "2"})
	require.Len(t, s.entries, 1, "duplicate ID should be ignored")
	assert.Equal(t, "a1", s.entries[0].Action, "first entry should be kept")
}

func TestGetEntries_ReturnsCopy(t *testing.T) {
	s := newTestAuditService()
	s.logEntry(&AuditEntry{ID: "e1", Timestamp: "t1", Action: "a1", Actor: "u1", ResourceType: "r", ResourceID: "1"})
	entries := s.getEntries()
	assert.Len(t, entries, 1)
	entries[0].Action = "modified"
	assert.Equal(t, "a1", s.entries[0].Action, "original should not be modified")
}

func TestHandleCreateEntry_ValidRequest(t *testing.T) {
	s := newTestAuditService()
	body := `{"action":"login","actor":"admin","resource_type":"user","resource_id":"u-1"}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/audit/log", bytes.NewReader([]byte(body)))
	s.handleCreateEntry(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	var entry AuditEntry
	err := json.Unmarshal(rr.Body.Bytes(), &entry)
	require.NoError(t, err)
	assert.Equal(t, "login", entry.Action)
	assert.Equal(t, "admin", entry.Actor)
	assert.NotEqual(t, "", entry.ID)
	assert.NotEqual(t, "", entry.Hash)
	assert.NotEqual(t, "", entry.Timestamp)
}

func TestHandleCreateEntry_MissingFields(t *testing.T) {
	s := newTestAuditService()
	tests := []struct {
		name string
		body string
	}{
		{"empty body", `{}`},
		{"missing action", `{"actor":"admin","resource_type":"user","resource_id":"u-1"}`},
		{"missing actor", `{"action":"login","resource_type":"user","resource_id":"u-1"}`},
		{"missing resource_type", `{"action":"login","actor":"admin","resource_id":"u-1"}`},
		{"missing resource_id", `{"action":"login","actor":"admin","resource_type":"user"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/audit/log", bytes.NewReader([]byte(tt.body)))
			s.handleCreateEntry(rr, req)
			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

func TestHandleCreateEntry_InvalidJSON(t *testing.T) {
	s := newTestAuditService()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/audit/log", bytes.NewReader([]byte(`not-json`)))
	s.handleCreateEntry(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleGetChain_Empty(t *testing.T) {
	s := newTestAuditService()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit/chain", nil)
	s.handleGetChain(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var entries []AuditEntry
	err := json.Unmarshal(rr.Body.Bytes(), &entries)
	require.NoError(t, err)
	assert.Len(t, entries, 0)
}

func TestHandleGetChain_WithEntries(t *testing.T) {
	s := newTestAuditService()
	s.logEntry(&AuditEntry{ID: "e1", Timestamp: "t1", Action: "create", Actor: "admin", ResourceType: "camera", ResourceID: "cam-1"})
	s.logEntry(&AuditEntry{ID: "e2", Timestamp: "t2", Action: "delete", Actor: "admin", ResourceType: "camera", ResourceID: "cam-1"})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit/chain", nil)
	s.handleGetChain(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var entries []AuditEntry
	err := json.Unmarshal(rr.Body.Bytes(), &entries)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "create", entries[0].Action)
	assert.Equal(t, "delete", entries[1].Action)
	assert.Equal(t, entries[0].Hash, entries[1].PreviousHash)
}

func TestHandleVerify_Empty(t *testing.T) {
	s := newTestAuditService()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit/verify", nil)
	s.handleVerify(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var result struct {
		Valid bool `json:"valid"`
		Count int  `json:"count"`
	}
	err := json.Unmarshal(rr.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.True(t, result.Valid)
	assert.Equal(t, 0, result.Count)
}

func TestHandleVerify_ValidChain(t *testing.T) {
	s := newTestAuditService()
	s.logEntry(&AuditEntry{ID: "e1", Timestamp: "t1", Action: "create", Actor: "admin", ResourceType: "camera", ResourceID: "cam-1"})
	s.logEntry(&AuditEntry{ID: "e2", Timestamp: "t2", Action: "update", Actor: "admin", ResourceType: "camera", ResourceID: "cam-1"})
	s.logEntry(&AuditEntry{ID: "e3", Timestamp: "t3", Action: "delete", Actor: "admin", ResourceType: "camera", ResourceID: "cam-1"})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit/verify", nil)
	s.handleVerify(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var result struct {
		Valid bool `json:"valid"`
		Count int  `json:"count"`
	}
	err := json.Unmarshal(rr.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.True(t, result.Valid)
	assert.Equal(t, 3, result.Count)
}

func TestHandleVerify_TamperedEntry(t *testing.T) {
	s := newTestAuditService()
	s.logEntry(&AuditEntry{ID: "e1", Timestamp: "t1", Action: "create", Actor: "admin", ResourceType: "camera", ResourceID: "cam-1"})
	s.logEntry(&AuditEntry{ID: "e2", Timestamp: "t2", Action: "update", Actor: "admin", ResourceType: "camera", ResourceID: "cam-1"})

	// Tamper with the second entry
	s.entries[1].Action = "delete"

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit/verify", nil)
	s.handleVerify(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var result struct {
		Valid bool `json:"valid"`
	}
	err := json.Unmarshal(rr.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.False(t, result.Valid, "tampered chain should be invalid")
}

func TestHandleVerify_TamperedPreviousHash(t *testing.T) {
	s := newTestAuditService()
	s.logEntry(&AuditEntry{ID: "e1", Timestamp: "t1", Action: "create", Actor: "admin", ResourceType: "camera", ResourceID: "cam-1"})
	s.logEntry(&AuditEntry{ID: "e2", Timestamp: "t2", Action: "update", Actor: "admin", ResourceType: "camera", ResourceID: "cam-1"})

	// Break the chain by modifying previous hash
	s.entries[1].PreviousHash = "0000000000000000000000000000000000000000000000000000000000000000"

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit/verify", nil)
	s.handleVerify(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var result struct {
		Valid bool `json:"valid"`
	}
	err := json.Unmarshal(rr.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.False(t, result.Valid, "broken chain should be invalid")
}

func TestJSONError_Audit(t *testing.T) {
	rr := httptest.NewRecorder()
	jsonError(rr, "bad request", http.StatusBadRequest)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	assert.Equal(t, "bad request", body["error"])
}
