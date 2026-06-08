package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/golang-jwt/jwt/v5"
)

type OIDCConfig struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	JWKSUri               string   `json:"jwks_uri"`
	UserInfoEndpoint      string   `json:"userinfo_endpoint"`
	ScopesSupported       []string `json:"scopes_supported"`
}

type OIDCTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

type OIDCUserInfo struct {
	Sub           string `json:"sub"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	PreferredName string `json:"preferred_username"`
}

func fetchOIDCConfig(issuerURL string) (*OIDCConfig, error) {
	wellKnown := strings.TrimSuffix(issuerURL, "/") + "/.well-known/openid-configuration"
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(wellKnown)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OIDC config: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read OIDC config: %w", err)
	}

	var config OIDCConfig
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, fmt.Errorf("failed to parse OIDC config: %w", err)
	}

	return &config, nil
}

func generateOIDCState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateCodeVerifier() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func computeCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func (s *AuthService) initiateOIDCLogin(w http.ResponseWriter, r *http.Request, provider SSOProvider, redirectURI string) {
	oidcConfig, err := fetchOIDCConfig(provider.IssuerURL)
	if err != nil {
		s.logger.Error("Failed to fetch OIDC config", "error", err)
		jsonError(w, "failed to configure OIDC provider", http.StatusInternalServerError)
		return
	}

	state := generateOIDCState()
	nonce := generateOIDCState()
	codeVerifier := generateCodeVerifier()
	codeChallenge := computeCodeChallenge(codeVerifier)

	scopes := provider.Scopes
	if scopes == "" {
		scopes = "openid profile email"
	}

	authURL, _ := url.Parse(oidcConfig.AuthorizationEndpoint)
	query := authURL.Query()
	query.Set("client_id", provider.ClientID)
	query.Set("response_type", "code")
	query.Set("scope", scopes)
	query.Set("redirect_uri", redirectURI)
	query.Set("state", state)
	query.Set("nonce", nonce)
	query.Set("code_challenge_method", "S256")
	query.Set("code_challenge", codeChallenge)
	authURL.RawQuery = query.Encode()

	// Store state and verifier (in memory - production would use a cache)
	http.SetCookie(w, &http.Cookie{
		Name:     "oidc_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // Set to true in production
		MaxAge:   600,
		SameSite: http.SameSiteLaxMode,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"authorization_url": authURL.String(),
		"state":             state,
	})
}

func (s *AuthService) handleOIDCCallback(w http.ResponseWriter, r *http.Request, provider SSOProvider) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" {
		jsonError(w, "authorization code required", http.StatusBadRequest)
		return
	}
	if state == "" {
		s.logger.Warn("OIDC callback missing state parameter")
	}

	oidcConfig, err := fetchOIDCConfig(provider.IssuerURL)
	if err != nil {
		s.logger.Error("Failed to fetch OIDC config", "error", err)
		jsonError(w, "failed to configure OIDC provider", http.StatusInternalServerError)
		return
	}

	// Exchange code for token
	clientSecret := common.MustDecrypt(provider.ClientSecretEncrypted)

	tokenReq := url.Values{}
	tokenReq.Set("grant_type", "authorization_code")
	tokenReq.Set("code", code)
	tokenReq.Set("redirect_uri", "/auth/sso/"+provider.Name+"/callback")
	tokenReq.Set("client_id", provider.ClientID)
	tokenReq.Set("client_secret", clientSecret)

	tokenResp, err := http.PostForm(oidcConfig.TokenEndpoint, tokenReq)
	if err != nil {
		s.logger.Error("Failed to exchange code for token", "error", err)
		jsonError(w, "failed to exchange authorization code", http.StatusInternalServerError)
		return
	}
	defer tokenResp.Body.Close()

	body, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		jsonError(w, "failed to read token response", http.StatusInternalServerError)
		return
	}

	if tokenResp.StatusCode != http.StatusOK {
		jsonError(w, fmt.Sprintf("token exchange failed: %s", string(body)), http.StatusInternalServerError)
		return
	}

	var oidcToken OIDCTokenResponse
	if err := json.Unmarshal(body, &oidcToken); err != nil {
		jsonError(w, "failed to parse token response", http.StatusInternalServerError)
		return
	}

	// Validate ID token
	idToken, err := jwt.Parse(oidcToken.IDToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); ok {
			return []byte(clientSecret), nil
		}
		if _, ok := token.Method.(*jwt.SigningMethodRSA); ok {
			kid, _ := token.Header["kid"].(string)
			return fetchJWKSPublicKey(oidcConfig.JWKSUri, kid)
		}
		return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
	})
	if err != nil {
		s.logger.Error("Failed to validate ID token", "error", err)
		jsonError(w, "invalid ID token", http.StatusUnauthorized)
		return
	}

	claims, ok := idToken.Claims.(jwt.MapClaims)
	if !ok || !idToken.Valid {
		jsonError(w, "invalid ID token claims", http.StatusUnauthorized)
		return
	}

	sub, _ := claims["sub"].(string)
	email, _ := claims["email"].(string)
	preferredUsername, _ := claims["preferred_username"].(string)

	if sub == "" {
		jsonError(w, "missing subject in ID token", http.StatusUnauthorized)
		return
	}

	username := preferredUsername
	if username == "" && email != "" {
		parts := strings.Split(email, "@")
		username = parts[0]
	}
	if username == "" {
		username = "oidc_" + sub[:8]
	}

	user, err := s.findOrCreateSSOUser(provider.ID, sub, email, username)
	if err != nil {
		s.logger.Error("Failed to process OIDC user", "error", err)
		jsonError(w, "failed to process authentication", http.StatusInternalServerError)
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

func fetchJWKSPublicKey(jwksURI, keyID string) (interface{}, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(jwksURI)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read JWKS: %w", err)
	}

	var jwks struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
			Alg string `json:"alg"`
			Use string `json:"use"`
		} `json:"keys"`
	}

	if err := json.Unmarshal(body, &jwks); err != nil {
		return nil, fmt.Errorf("failed to parse JWKS: %w", err)
	}

	for _, key := range jwks.Keys {
		if key.Kid == keyID || keyID == "" {
			if key.Kty == "RSA" {
				nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
				if err != nil {
					return nil, fmt.Errorf("failed to decode JWK modulus: %w", err)
				}
				eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
				if err != nil {
					return nil, fmt.Errorf("failed to decode JWK exponent: %w", err)
				}

				e := 0
				for _, b := range eBytes {
					e = e*256 + int(b)
				}

				nInt := new(big.Int).SetBytes(nBytes)
				return &rsa.PublicKey{
					N: nInt,
					E: e,
				}, nil
			}
			if key.Kty == "oct" {
				nBytes, _ := base64.RawURLEncoding.DecodeString(key.N)
				return nBytes, nil
			}
		}
	}

	return nil, fmt.Errorf("no matching JWK found for kid: %s", keyID)
}

func (s *AuthService) initiateSAMLLogin(w http.ResponseWriter, r *http.Request, provider SSOProvider, redirectURI string) {
	// Generate SAML AuthnRequest (simplified)
	samlRequest := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<saml2p:AuthnRequest xmlns:saml2p="urn:oasis:names:tc:SAML:2.0:protocol"
    xmlns:saml2="urn:oasis:names:tc:SAML:2.0:assertion"
    ID="_%s" Version="2.0"
    IssueInstant="%s"
    Destination="%s"
    AssertionConsumerServiceURL="%s">
    <saml2:Issuer>%s</saml2:Issuer>
    <saml2p:NameIDPolicy Format="urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress"/>
</saml2p:AuthnRequest>`,
		generateOIDCState(),
		time.Now().UTC().Format(time.RFC3339),
		provider.ACSUrl,
		redirectURI,
		provider.EntityID)

	encodedRequest := base64.StdEncoding.EncodeToString([]byte(samlRequest))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"saml_request": encodedRequest,
		"acs_url":      provider.ACSUrl,
		"relay_state":  redirectURI,
	})
}
