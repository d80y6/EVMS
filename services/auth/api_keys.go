package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/google/uuid"
)

type APIKey struct {
	ID         string     `db:"id" json:"id"`
	UserID     string     `db:"user_id" json:"user_id"`
	Name       string     `db:"name" json:"name"`
	KeyPrefix  string     `db:"key_prefix" json:"key_prefix"`
	KeyHash    string     `db:"key_hash" json:"-"`
	Scopes     string     `db:"scopes" json:"scopes"`
	CameraIDs  string      `db:"camera_ids" json:"camera_ids,omitempty"`
	ExpiresAt  *time.Time `db:"expires_at" json:"expires_at,omitempty"`
	LastUsedAt *time.Time `db:"last_used_at" json:"last_used_at,omitempty"`
	Active     bool       `db:"active" json:"active"`
	CreatedAt  time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time  `db:"updated_at" json:"updated_at"`
}

type createAPIKeyRequest struct {
	Name      string   `json:"name"`
	Scopes    string   `json:"scopes"`
	CameraIDs []string `json:"camera_ids,omitempty"`
	ExpiresIn string   `json:"expires_in,omitempty"`
}

type createAPIKeyResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	KeyPrefix string `json:"key_prefix"`
	Scopes    string `json:"scopes"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type apiKeyResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	KeyPrefix string     `json:"key_prefix"`
	Scopes    string     `json:"scopes"`
	Active    bool       `json:"active"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func generateAPIKey() (string, string, string, error) {
	prefix := "evms_" + uuid.New().String()[:8]

	keyBytes := make([]byte, 32)
	_, err := rand.Read(keyBytes)
	if err != nil {
		return "", "", "", err
	}
	key := hex.EncodeToString(keyBytes)

	fullKey := prefix + "_" + key

	hash := sha256.Sum256([]byte(fullKey))

	return fullKey, prefix, hex.EncodeToString(hash[:]), nil
}

func validateAPIKeyScope(key *APIKey, requiredScope string) bool {
	if key.Scopes == "admin" {
		return true
	}
	scopes := strings.Split(key.Scopes, ",")
	for _, s := range scopes {
		if strings.TrimSpace(s) == requiredScope {
			return true
		}
	}
	return false
}

func (s *AuthService) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := r.Context().Value(common.UserKey).(string)
	role, _ := r.Context().Value(common.RoleKey).(string)

	var user User
	err := s.db.Get(&user,
		"SELECT id FROM users WHERE username = $1 AND active = true AND deleted_at IS NULL",
		username)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}

	var keys []APIKey
	if role == "admin" {
		err = s.db.Select(&keys,
			"SELECT id, user_id, name, key_prefix, scopes, camera_ids, expires_at, last_used_at, active, created_at FROM api_keys ORDER BY created_at DESC")
	} else {
		err = s.db.Select(&keys,
			"SELECT id, user_id, name, key_prefix, scopes, camera_ids, expires_at, last_used_at, active, created_at FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC",
			user.ID)
	}
	if err != nil {
		s.logger.Error("Failed to list API keys", "error", err)
		jsonError(w, "failed to list API keys", http.StatusInternalServerError)
		return
	}

	resp := make([]apiKeyResponse, len(keys))
	for i, k := range keys {
		resp[i] = apiKeyResponse{
			ID:         k.ID,
			Name:       k.Name,
			KeyPrefix:  k.KeyPrefix,
			Scopes:     k.Scopes,
			Active:     k.Active,
			LastUsedAt: k.LastUsedAt,
			ExpiresAt:  k.ExpiresAt,
			CreatedAt:  k.CreatedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"api_keys": resp})
}

func (s *AuthService) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := r.Context().Value(common.UserKey).(string)

	var user User
	err := s.db.Get(&user,
		"SELECT id FROM users WHERE username = $1 AND active = true AND deleted_at IS NULL",
		username)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}

	var req createAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}

	if req.Scopes == "" {
		req.Scopes = "read"
	}

	var expiresAt *time.Time
	if req.ExpiresIn != "" {
		duration, err := time.ParseDuration(req.ExpiresIn)
		if err != nil {
			jsonError(w, "invalid expires_in duration", http.StatusBadRequest)
			return
		}
		t := time.Now().Add(duration)
		expiresAt = &t
	}

	fullKey, prefix, hash, err := generateAPIKey()
	if err != nil {
		s.logger.Error("Failed to generate API key", "error", err)
		jsonError(w, "failed to generate API key", http.StatusInternalServerError)
		return
	}

	var cameraIDs interface{}
	if len(req.CameraIDs) > 0 {
		cameraIDs = req.CameraIDs
	}

	var keyID string
	err = s.db.QueryRow(
		`INSERT INTO api_keys (user_id, name, key_prefix, key_hash, scopes, camera_ids, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		user.ID, req.Name, prefix, hash, req.Scopes, cameraIDs, expiresAt).Scan(&keyID)
	if err != nil {
		s.logger.Error("Failed to save API key", "error", err)
		jsonError(w, "failed to create API key", http.StatusInternalServerError)
		return
	}

	resp := createAPIKeyResponse{
		ID:        keyID,
		Name:      req.Name,
		Key:       fullKey,
		KeyPrefix: prefix,
		Scopes:    req.Scopes,
	}
	if expiresAt != nil {
		resp.ExpiresAt = expiresAt.Format(time.RFC3339)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (s *AuthService) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	keyID := extractIDFromPath(r.URL.Path, "/auth/api-keys/")
	if keyID == "" {
		jsonError(w, "key id required", http.StatusBadRequest)
		return
	}

	username := r.Context().Value(common.UserKey).(string)
	role, _ := r.Context().Value(common.RoleKey).(string)

	var user User
	err := s.db.Get(&user,
		"SELECT id FROM users WHERE username = $1 AND active = true AND deleted_at IS NULL",
		username)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}

	var result interface{}
	if role == "admin" {
		result, err = s.db.Exec(
			"UPDATE api_keys SET active = false, updated_at = NOW() WHERE id = $1 AND active = true",
			keyID)
	} else {
		result, err = s.db.Exec(
			"UPDATE api_keys SET active = false, updated_at = NOW() WHERE id = $1 AND user_id = $2 AND active = true",
			keyID, user.ID)
	}

	if err != nil {
		s.logger.Error("Failed to revoke API key", "error", err)
		jsonError(w, "failed to revoke API key", http.StatusInternalServerError)
		return
	}

	rows, _ := result.(interface{ RowsAffected() (int64, error) }).RowsAffected()
	if rows == 0 {
		jsonError(w, "API key not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "revoked"})
}

type apiKeyAuthResponse struct {
	UserID  string `json:"user_id"`
	Scopes  string `json:"scopes"`
	Role    string `json:"role"`
}

func (s *AuthService) authenticateWithAPIKey(apiKey string) (*apiKeyAuthResponse, error) {
	parts := strings.SplitN(apiKey, "_", 3)
	if len(parts) < 3 || parts[0] != "evms" {
		return nil, fmt.Errorf("invalid API key format")
	}
	prefix := parts[0] + "_" + parts[1]

	hash := sha256.Sum256([]byte(apiKey))
	keyHash := hex.EncodeToString(hash[:])

	var key APIKey
	err := s.db.Get(&key,
		"SELECT id, user_id, name, key_hash, scopes, camera_ids, expires_at, active FROM api_keys WHERE key_prefix = $1 AND active = true",
		prefix)
	if err != nil {
		return nil, fmt.Errorf("API key not found")
	}

	if !key.Active {
		return nil, fmt.Errorf("API key is inactive")
	}

	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		s.db.Exec("UPDATE api_keys SET active = false, updated_at = NOW() WHERE id = $1", key.ID)
		return nil, fmt.Errorf("API key has expired")
	}

	if keyHash != key.KeyHash {
		return nil, fmt.Errorf("API key mismatch")
	}

	s.db.Exec("UPDATE api_keys SET last_used_at = NOW() WHERE id = $1", key.ID)

	var user User
	err = s.db.Get(&user,
		"SELECT id, role FROM users WHERE id = $1 AND active = true AND deleted_at IS NULL",
		key.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	return &apiKeyAuthResponse{
		UserID: user.ID,
		Scopes: key.Scopes,
		Role:   user.Role,
	}, nil
}
