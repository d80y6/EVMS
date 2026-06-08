package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type SSOProvider struct {
	ID                   string `db:"id"`
	Name                 string `db:"name"`
	ProviderType         string `db:"provider_type"`
	IssuerURL            string `db:"issuer_url"`
	ClientID             string `db:"client_id"`
	ClientSecretEncrypted string `db:"client_secret_encrypted"`
	Scopes               string `db:"scopes"`
	MetadataURL          string `db:"metadata_url"`
	ACSUrl               string `db:"acs_url"`
	EntityID             string `db:"entity_id"`
	Certificate          string `db:"certificate"`
	Enabled              bool   `db:"enabled"`
}

type SSOIdentity struct {
	ID         string    `db:"id"`
	UserID     string    `db:"user_id"`
	ProviderID string    `db:"provider_id"`
	ExternalID string    `db:"external_id"`
	Email      string    `db:"email"`
	CreatedAt  time.Time `db:"created_at"`
}

type ssoProviderResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	Enabled      bool   `json:"enabled"`
}

type oidcConfigResponse struct {
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	JWKSUri               string   `json:"jwks_uri"`
	Issuer                string   `json:"issuer"`
	ScopesSupported       []string `json:"scopes_supported"`
}

type ssoAuthorizeRequest struct {
	RedirectURI string `json:"redirect_uri"`
}

func (s *AuthService) handleSSOProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var providers []SSOProvider
	err := s.db.Select(&providers,
		"SELECT id, name, provider_type, enabled FROM sso_providers WHERE enabled = true")
	if err != nil {
		s.logger.Error("Failed to list SSO providers", "error", err)
		jsonError(w, "failed to list providers", http.StatusInternalServerError)
		return
	}

	resp := make([]ssoProviderResponse, len(providers))
	for i, p := range providers {
		resp[i] = ssoProviderResponse{
			ID:           p.ID,
			Name:         p.Name,
			ProviderType: p.ProviderType,
			Enabled:      p.Enabled,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"providers": resp})
}

func (s *AuthService) handleSSOAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	providerName := extractIDFromPath(r.URL.Path, "/auth/sso/")
	providerName = strings.TrimSuffix(providerName, "/authorize")
	if providerName == "" {
		jsonError(w, "provider name required", http.StatusBadRequest)
		return
	}

	var provider SSOProvider
	err := s.db.Get(&provider,
		"SELECT id, name, provider_type, issuer_url, client_id, client_secret_encrypted, scopes, acs_url, enabled FROM sso_providers WHERE name = $1 AND enabled = true",
		providerName)
	if err != nil {
		jsonError(w, "SSO provider not found", http.StatusNotFound)
		return
	}

	redirectURI := r.URL.Query().Get("redirect_uri")
	if redirectURI == "" {
		if r.Method == http.MethodPost {
			var req ssoAuthorizeRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				redirectURI = req.RedirectURI
			}
		}
		if redirectURI == "" {
			redirectURI = "/auth/sso/" + providerName + "/callback"
		}
	}

	if provider.ProviderType == "oidc" {
		s.initiateOIDCLogin(w, r, provider, redirectURI)
	} else if provider.ProviderType == "saml" {
		s.initiateSAMLLogin(w, r, provider, redirectURI)
	} else {
		jsonError(w, "unsupported provider type", http.StatusBadRequest)
	}
}

func (s *AuthService) handleSSOCallback(w http.ResponseWriter, r *http.Request) {
	providerName := extractIDFromPath(r.URL.Path, "/auth/sso/")
	providerName = strings.TrimSuffix(providerName, "/callback")
	if providerName == "" {
		jsonError(w, "provider name required", http.StatusBadRequest)
		return
	}

	var provider SSOProvider
	err := s.db.Get(&provider,
		"SELECT id, name, provider_type, issuer_url, client_id, client_secret_encrypted, scopes, enabled FROM sso_providers WHERE name = $1 AND enabled = true",
		providerName)
	if err != nil {
		jsonError(w, "SSO provider not found", http.StatusNotFound)
		return
	}

	if provider.ProviderType == "oidc" {
		s.handleOIDCCallback(w, r, provider)
	} else {
		jsonError(w, "callback not supported for this provider type", http.StatusBadRequest)
	}
}

func (s *AuthService) handleSAMLACS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	providerName := extractIDFromPath(r.URL.Path, "/auth/sso/")
	providerName = strings.TrimSuffix(providerName, "/acs")
	if providerName == "" {
		jsonError(w, "provider name required", http.StatusBadRequest)
		return
	}

	var provider SSOProvider
	err := s.db.Get(&provider,
		"SELECT id, name, provider_type, entity_id, certificate, enabled FROM sso_providers WHERE name = $1 AND enabled = true",
		providerName)
	if err != nil {
		jsonError(w, "SSO provider not found", http.StatusNotFound)
		return
	}

	samlResponse := r.FormValue("SAMLResponse")
	if samlResponse == "" {
		jsonError(w, "SAMLResponse required", http.StatusBadRequest)
		return
	}

	// Decode base64 SAML response
	decoded, err := base64.StdEncoding.DecodeString(samlResponse)
	if err != nil {
		jsonError(w, "invalid SAML response encoding", http.StatusBadRequest)
		return
	}

	// Extract NameID from SAML response (simplified - in production use XML parsing)
	nameID := extractNameIDFromSAML(string(decoded))
	if nameID == "" {
		jsonError(w, "could not extract identity from SAML response", http.StatusBadRequest)
		return
	}

	email := nameID
	if strings.Contains(nameID, "@") {
		email = nameID
	}

	user, err := s.findOrCreateSSOUser(provider.ID, nameID, email, providerName)
	if err != nil {
		s.logger.Error("Failed to process SSO user", "error", err)
		jsonError(w, "failed to process SSO authentication", http.StatusInternalServerError)
		return
	}

	token, err := s.generateToken(*user)
	if err != nil {
		jsonError(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LoginResponse{Token: token})
}

func extractNameIDFromSAML(samlXML string) string {
	// Simple extraction - production would use proper XML parsing
	startTag := "<saml2:NameID"
	endTag := "</saml2:NameID>"

	start := strings.Index(samlXML, startTag)
	if start < 0 {
		startTag = "<NameID"
		start = strings.Index(samlXML, startTag)
		if start < 0 {
			return ""
		}
	}

	valueStart := strings.Index(samlXML[start:], ">")
	if valueStart < 0 {
		return ""
	}
	valueStart += start + 1

	valueEnd := strings.Index(samlXML[valueStart:], endTag)
	if valueEnd < 0 {
		// Try without namespace
		endTag = "</NameID>"
		valueEnd = strings.Index(samlXML[valueStart:], endTag)
		if valueEnd < 0 {
			return ""
		}
	}

	return strings.TrimSpace(samlXML[valueStart : valueStart+valueEnd])
}

func (s *AuthService) handleAdminSSOProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var providers []SSOProvider
		err := s.db.Select(&providers,
			"SELECT id, name, provider_type, issuer_url, client_id, scopes, metadata_url, acs_url, entity_id, enabled, created_at FROM sso_providers ORDER BY created_at DESC")
		if err != nil {
			jsonError(w, "failed to list providers", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"providers": providers})

	case http.MethodPost:
		var req struct {
			Name        string `json:"name"`
			ProviderType string `json:"provider_type"`
			IssuerURL   string `json:"issuer_url"`
			ClientID    string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
			Scopes      string `json:"scopes"`
			MetadataURL string `json:"metadata_url"`
			ACSUrl      string `json:"acs_url"`
			EntityID    string `json:"entity_id"`
			Certificate string `json:"certificate"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.ProviderType == "" {
			jsonError(w, "name and provider_type required", http.StatusBadRequest)
			return
		}

		encryptedSecret := ""
		if req.ClientSecret != "" {
			encryptedSecret = common.MustEncrypt(req.ClientSecret)
		}

		_, err := s.db.Exec(
			`INSERT INTO sso_providers (name, provider_type, issuer_url, client_id, client_secret_encrypted, scopes, metadata_url, acs_url, entity_id, certificate, enabled)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, true)`,
			req.Name, req.ProviderType, req.IssuerURL, req.ClientID, encryptedSecret,
			req.Scopes, req.MetadataURL, req.ACSUrl, req.EntityID, req.Certificate)
		if err != nil {
			jsonError(w, "failed to create provider", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "created"})

	default:
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func generateSSOState() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func jwtFromClaims(claims map[string]interface{}) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims(claims))
	tokenString, _ := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	return tokenString
}

func (s *AuthService) findOrCreateSSOUser(providerID, externalID, email, defaultUsername string) (*User, error) {
	var identity SSOIdentity
	err := s.db.Get(&identity,
		"SELECT id, user_id, provider_id, external_id, email FROM sso_identities WHERE provider_id = $1 AND external_id = $2",
		providerID, externalID)
	if err == nil && identity.UserID != "" {
		var user User
		err = s.db.Get(&user,
			"SELECT id, username, password_hash, role, tenant_id, active FROM users WHERE id = $1 AND active = true AND deleted_at IS NULL",
			identity.UserID)
		if err == nil {
			return &user, nil
		}
	}

	username := defaultUsername
	if email != "" {
		parts := strings.Split(email, "@")
		if len(parts) > 0 {
			username = parts[0]
		}
	}

	// Ensure unique username
	baseUsername := username
	for i := 2; ; i++ {
		var exists int
		s.db.Get(&exists, "SELECT COUNT(*) FROM users WHERE username = $1", username)
		if exists == 0 {
			break
		}
		username = fmt.Sprintf("%s_%d", baseUsername, i)
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte("sso_"+externalID), bcrypt.DefaultCost)

	var userID string
	err = s.db.QueryRow(
		"INSERT INTO users (username, email, password_hash, role, active) VALUES ($1, $2, $3, 'viewer', true) RETURNING id",
		username, email, string(hash)).Scan(&userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	_, err = s.db.Exec(
		"INSERT INTO sso_identities (user_id, provider_id, external_id, email) VALUES ($1, $2, $3, $4)",
		userID, providerID, externalID, email)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSO identity: %w", err)
	}

	return &User{ID: userID, Username: username, Role: "viewer", Active: true}, nil
}
