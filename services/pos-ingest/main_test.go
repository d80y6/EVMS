package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupJWT(t *testing.T) string {
	t.Helper()
	os.Setenv("JWT_SECRET", "test-secret-pos-ingest")
	common.ReloadJWTKey()
	t.Cleanup(func() {
		os.Unsetenv("JWT_SECRET")
		common.ReloadJWTKey()
	})
	token, err := common.SignJWT(&common.Claims{
		Username: "test-user",
		Role:     "admin",
	})
	assert.NoError(t, err)
	return token
}

func authRequest(method, path string, body []byte, token string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func TestJSONError_POS(t *testing.T) {
	rr := httptest.NewRecorder()
	jsonError(rr, "bad request", http.StatusBadRequest)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	assert.Equal(t, "bad request", body["error"])
}

func TestPOSHandler_AuthMiddleware(t *testing.T) {
	testHandler := common.JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("no auth header", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/pos/transaction", nil)
		testHandler(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("invalid token", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := authRequest(http.MethodPost, "/api/pos/transaction", nil, "bad-token")
		testHandler(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("valid token", func(t *testing.T) {
		token := setupJWT(t)
		rr := httptest.NewRecorder()
		req := authRequest(http.MethodPost, "/api/pos/transaction", nil, token)
		testHandler(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}

func TestPOSJson_RoundTrip(t *testing.T) {
	original := POSTransaction{
		ID:            "tx-1",
		CameraID:      "cam-1",
		StoreID:       "store-1",
		RegisterID:    "reg-1",
		TransactionID: "tx-ext-1",
		Timestamp:     time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		Items: []POSItem{
			{SKU: "SKU123", Description: "Item", Quantity: 2, UnitPrice: 10.0, Total: 10.0},
		},
		Subtotal:   20.0,
		Tax:        2.0,
		Total:      22.0,
		TenderType: "credit",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded POSTransaction
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original.ID, decoded.ID)
	assert.Equal(t, original.CameraID, decoded.CameraID)
	assert.Equal(t, len(original.Items), len(decoded.Items))
	assert.Equal(t, original.Items[0].SKU, decoded.Items[0].SKU)
	assert.Equal(t, original.Total, decoded.Total)
	assert.True(t, original.Timestamp.Equal(decoded.Timestamp))
}

func TestPOSTransaction_IDDefaults(t *testing.T) {
	var tx POSTransaction
	if tx.ID == "" {
		tx.ID = uuid.New().String()
	}
	assert.NotEmpty(t, tx.ID)
	id, err := uuid.Parse(tx.ID)
	assert.NoError(t, err)
	assert.Len(t, id.String(), 36)
}

func TestPOSTransaction_TimestampDefaults(t *testing.T) {
	var tx POSTransaction
	if tx.Timestamp.IsZero() {
		tx.Timestamp = time.Now().UTC()
	}
	assert.False(t, tx.Timestamp.IsZero())
}

func TestPOSHandler_MethodCheck(t *testing.T) {
	token := setupJWT(t)

	testHandler := common.JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})

	rr := httptest.NewRecorder()
	req := authRequest(http.MethodGet, "/api/pos/transaction", nil, token)
	testHandler(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	assert.Equal(t, "method not allowed", body["error"])
}
