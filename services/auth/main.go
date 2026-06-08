package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/go-ldap/ldap/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// AuthConfig holds configuration for the auth service
type AuthConfig struct {
	HTTPAddr    string
	DBURL       string
	JWTSecret   []byte
	TokenExpiry time.Duration

	LDAPEnabled  bool
	LDAPHost     string
	LDAPPort     int
	LDAPBaseDN   string
	LDAPBindDN   string
	LDAPPassword string
	LDAPFilter   string

	PasswordPolicy PasswordPolicy
	SessionLimit   int
}

// DefaultAuthConfig returns a configuration with sensible defaults
func DefaultAuthConfig() AuthConfig {
	policy := DefaultPasswordPolicy()
	return AuthConfig{
		HTTPAddr:    ":8081",
		DBURL:       os.Getenv("DB_URL"),
		JWTSecret:   []byte(os.Getenv("JWT_SECRET")),
		TokenExpiry: 24 * time.Hour,

		LDAPEnabled:  os.Getenv("LDAP_ENABLED") == "true",
		LDAPHost:     common.GetEnv("LDAP_HOST", "localhost"),
		LDAPPort:     389,
		LDAPBaseDN:   common.GetEnv("LDAP_BASE_DN", "dc=example,dc=com"),
		LDAPBindDN:   common.GetEnv("LDAP_BIND_DN", ""),
		LDAPPassword: os.Getenv("LDAP_PASSWORD"),
		LDAPFilter:   common.GetEnv("LDAP_FILTER", "(uid=%s)"),

		PasswordPolicy: policy,
		SessionLimit:   5,
	}
}

// Validate checks if the configuration is valid
func (c *AuthConfig) Validate() error {
	if len(c.JWTSecret) == 0 {
		return errors.New("JWT_SECRET environment variable is not set")
	}
	return nil
}

// User represents a user in the database
type User struct {
	ID           string     `db:"id"`
	Username     string     `db:"username"`
	PasswordHash string     `db:"password_hash"`
	Role         string     `db:"role"`
	TenantID     *string    `db:"tenant_id"`
	Active       bool       `db:"active"`
	CreatedAt    time.Time  `db:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at"`
	DeletedAt    *time.Time `db:"deleted_at"`
}

// UserResponse is the public representation of a user (no password hash)
type UserResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateUserRequest represents a request to create a user
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// UpdateUserRequest represents a request to update a user
type UpdateUserRequest struct {
	Role     string `json:"role,omitempty"`
	Password string `json:"password,omitempty"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	MFARequired  bool   `json:"mfa_required,omitempty"`
	MFAToken     string `json:"mfa_token,omitempty"`
}

// Session represents a user session in the database
type Session struct {
	ID               string    `db:"id"`
	UserID           string    `db:"user_id"`
	RefreshTokenHash string    `db:"refresh_token_hash"`
	IPAddress        string    `db:"ip_address"`
	UserAgent        string    `db:"user_agent"`
	CreatedAt        time.Time `db:"created_at"`
	ExpiresAt        time.Time `db:"expires_at"`
	Active           bool      `db:"active"`
}

// SessionResponse is the public representation of a session
type SessionResponse struct {
	ID        string    `json:"id"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Active    bool      `json:"active"`
}

type refreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type refreshTokenResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
}

type revokeSessionRequest struct {
	SessionID string `json:"session_id"`
}

// AuthService handles authentication operations
type AuthService struct {
	logger        *slog.Logger
	db            *sqlx.DB
	config        AuthConfig
	healthHandler *common.HealthHandler
}

// NewAuthService creates a new auth service instance
func NewAuthService(ctx context.Context, config AuthConfig, logger *slog.Logger) (*AuthService, error) {
	cb := common.NewDBCircuitBreaker("auth")
	db, err := common.ConnectDBWithCircuitBreaker(ctx, "postgres", config.DBURL, cb)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	migrator := common.NewMigrator(db, common.GetEnv("MIGRATIONS_DIR", "/migrations"), logger)
	if err := migrator.Run(); err != nil {
		return nil, fmt.Errorf("migrations failed: %w", err)
	}

	adminUser := common.GetEnv("ADMIN_USERNAME", "")
	adminPass := common.GetEnv("ADMIN_PASSWORD", "")
	if adminUser != "" && adminPass != "" {
		var count int
		if err := db.Get(&count, "SELECT COUNT(*) FROM users"); err == nil && count == 0 {
			hash, err := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
			if err != nil {
				return nil, fmt.Errorf("failed to hash admin password: %w", err)
			}
			_, err = db.Exec(
				`INSERT INTO users (tenant_id, username, email, password_hash, role) VALUES ($1, $2, $3, $4, $5)`,
				nil, adminUser, adminUser+"@admin.local", string(hash), "admin",
			)
			if err != nil {
				return nil, fmt.Errorf("failed to create admin user: %w", err)
			}
			logger.Info("Default admin user created", "username", adminUser)
		}
	}

	return &AuthService{
		logger: logger,
		db:     db,
		config: config,
	}, nil
}

// Close gracefully shuts down the service
func (s *AuthService) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// handleLogin processes login requests
func (s *AuthService) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.Warn("Invalid login request", "error", err)
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		jsonError(w, "username and password required", http.StatusBadRequest)
		return
	}

	// Check account lockout
	locked, remaining, err := s.isAccountLocked(req.Username)
	if err != nil {
		s.logger.Warn("Failed to check account lockout", "error", err)
	}
	if locked {
		jsonError(w, fmt.Sprintf("account locked, try again in %.0f minutes", remaining.Minutes()), http.StatusTooManyRequests)
		return
	}

	token, refreshToken, mfaRequired, err := s.authenticateUser(r.Context(), req.Username, req.Password, r)
	if err != nil {
		s.logger.Warn("Authentication failed", "username", req.Username, "error", err)
		s.recordFailedAttempt(req.Username)
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	resp := LoginResponse{Token: token}
	if refreshToken != "" {
		resp.RefreshToken = refreshToken
	}
	if mfaRequired {
		resp.MFARequired = true
		resp.MFAToken = token
		resp.Token = ""
		resp.RefreshToken = ""
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// authenticateUser validates credentials and returns a JWT token
func (s *AuthService) authenticateUser(ctx context.Context, username, password string, r *http.Request) (string, string, bool, error) {
	var user *User
	var err error

	if s.config.LDAPEnabled {
		user, err = s.authenticateLDAP(ctx, username, password)
		if err != nil {
			s.logger.Warn("LDAP auth failed, falling back to local", "username", username, "error", err)
		}
	}

	if user == nil {
		var localUser User
		err = s.db.GetContext(ctx, &localUser,
			"SELECT id, username, password_hash, role, tenant_id FROM users WHERE username = $1 AND active = true AND deleted_at IS NULL",
			username)
		if err != nil {
			return "", "", false, fmt.Errorf("user not found: %w", err)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(localUser.PasswordHash), []byte(password)); err != nil {
			return "", "", false, errors.New("invalid password")
		}
		user = &localUser
	}

	// Check password expiry
	expired, err := s.checkPasswordExpiry(user.ID)
	if err == nil && expired {
		return "", "", false, fmt.Errorf("password has expired, please change your password")
	}

	// Check MFA requirement
	var mfaSettings MFASettings
	err = s.db.Get(&mfaSettings, "SELECT enabled FROM mfa_settings WHERE user_id = $1 AND enabled = true", user.ID)
	if err == nil && mfaSettings.Enabled {
		s.clearFailedAttempts(username)
		mfaToken, err := s.generateToken(*user)
		if err != nil {
			return "", "", false, fmt.Errorf("failed to generate MFA token: %w", err)
		}
		return mfaToken, "", true, nil
	}

	s.clearFailedAttempts(username)

	// Generate JWT
	token, err := s.generateToken(*user)
	if err != nil {
		return "", "", false, err
	}

	// Create session with refresh token
	ipAddress := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ipAddress = strings.Split(forwarded, ",")[0]
	}
	userAgent := r.Header.Get("User-Agent")

	refreshToken, err := s.createSession(user.ID, ipAddress, userAgent)
	if err != nil {
		s.logger.Warn("Failed to create session", "error", err)
		// Still return the token, just without a session
		return token, "", false, nil
	}

	return token, refreshToken, false, nil
}

func (s *AuthService) authenticateLDAP(ctx context.Context, username, password string) (*User, error) {
	conn, err := ldap.Dial("tcp", fmt.Sprintf("%s:%d", s.config.LDAPHost, s.config.LDAPPort))
	if err != nil {
		return nil, fmt.Errorf("ldap dial: %w", err)
	}
	defer conn.Close()

	if s.config.LDAPBindDN != "" {
		if err := conn.Bind(s.config.LDAPBindDN, s.config.LDAPPassword); err != nil {
			return nil, fmt.Errorf("ldap service bind: %w", err)
		}
	}

	filter := strings.ReplaceAll(s.config.LDAPFilter, "%s", ldap.EscapeFilter(username))
	searchReq := &ldap.SearchRequest{
		BaseDN:     s.config.LDAPBaseDN,
		Scope:      ldap.ScopeWholeSubtree,
		Filter:     filter,
		Attributes: []string{"uid", "mail", "cn"},
	}
	result, err := conn.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("ldap search: %w", err)
	}
	if len(result.Entries) == 0 {
		return nil, errors.New("user not found in ldap")
	}

	userDN := result.Entries[0].DN
	if err := conn.Bind(userDN, password); err != nil {
		return nil, errors.New("ldap bind failed: invalid password")
	}

	var user User
	err = s.db.GetContext(ctx, &user,
		"SELECT id, username, password_hash, role, tenant_id, active FROM users WHERE username = $1",
		username)
	if err != nil {
		var id string
		err = s.db.QueryRowContext(ctx,
			"INSERT INTO users (username, password_hash, role, active) VALUES ($1, '', 'viewer', true) RETURNING id",
			username).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("auto-provision user: %w", err)
		}
		user = User{ID: id, Username: username, Role: "viewer", Active: true}
	}

	return &user, nil
}

// generateToken creates a JWT token for a user
func (s *AuthService) generateToken(user User) (string, error) {
	expirationTime := time.Now().Add(s.config.TokenExpiry)
	var tenantID string
	if user.TenantID != nil {
		tenantID = *user.TenantID
	}
	claims := &common.Claims{
		Username: user.Username,
		Role:     user.Role,
		TenantID: tenantID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.config.JWTSecret)
}

func generateRefreshToken() (string, string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", "", err
	}
	rawToken := hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(rawToken))
	return rawToken, hex.EncodeToString(hash[:]), nil
}

func (s *AuthService) createSession(userID, ipAddress, userAgent string) (string, error) {
	// Check concurrent session limit
	var activeCount int
	s.db.Get(&activeCount,
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = $1 AND active = true AND expires_at > NOW()",
		userID)

	limit := s.config.SessionLimit
	if limit <= 0 {
		limit = 5
	}

	if activeCount >= limit {
		// Revoke oldest session
		var oldestID string
		err := s.db.Get(&oldestID,
			"SELECT id FROM user_sessions WHERE user_id = $1 AND active = true ORDER BY created_at ASC LIMIT 1",
			userID)
		if err == nil && oldestID != "" {
			s.db.Exec("UPDATE user_sessions SET active = false WHERE id = $1", oldestID)
		}
	}

	rawToken, hashedToken, err := generateRefreshToken()
	if err != nil {
		return "", err
	}

	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days

	_, err = s.db.Exec(
		`INSERT INTO user_sessions (user_id, refresh_token_hash, ip_address, user_agent, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, hashedToken, ipAddress, userAgent, expiresAt)
	if err != nil {
		return "", err
	}

	return rawToken, nil
}

func (s *AuthService) handleRefreshToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req refreshTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.RefreshToken == "" {
		jsonError(w, "refresh_token required", http.StatusBadRequest)
		return
	}

	hash := sha256.Sum256([]byte(req.RefreshToken))
	hashedToken := hex.EncodeToString(hash[:])

	var session Session
	err := s.db.Get(&session,
		"SELECT id, user_id, active, expires_at FROM user_sessions WHERE refresh_token_hash = $1 AND active = true",
		hashedToken)
	if err != nil {
		jsonError(w, "invalid refresh token", http.StatusUnauthorized)
		return
	}

	if time.Now().After(session.ExpiresAt) {
		s.db.Exec("UPDATE user_sessions SET active = false WHERE id = $1", session.ID)
		jsonError(w, "refresh token expired", http.StatusUnauthorized)
		return
	}

	// Invalidate old refresh token (rotation)
	s.db.Exec("UPDATE user_sessions SET active = false WHERE id = $1", session.ID)

	var user User
	err = s.db.Get(&user,
		"SELECT id, username, password_hash, role, tenant_id FROM users WHERE id = $1 AND active = true AND deleted_at IS NULL",
		session.UserID)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}

	token, err := s.generateToken(user)
	if err != nil {
		jsonError(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	// Create new session
	refreshToken, err := s.createSession(user.ID, session.IPAddress, session.UserAgent)
	if err != nil {
		jsonError(w, "failed to create new session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(refreshTokenResponse{
		Token:        token,
		RefreshToken: refreshToken,
	})
}

func (s *AuthService) handleListSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
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

	var sessions []Session
	err = s.db.Select(&sessions,
		"SELECT id, ip_address, user_agent, created_at, expires_at, active FROM user_sessions WHERE user_id = $1 ORDER BY created_at DESC",
		user.ID)
	if err != nil {
		s.logger.Error("Failed to list sessions", "error", err)
		jsonError(w, "failed to list sessions", http.StatusInternalServerError)
		return
	}

	resp := make([]SessionResponse, len(sessions))
	for i, s := range sessions {
		resp[i] = SessionResponse{
			ID:        s.ID,
			IPAddress: s.IPAddress,
			UserAgent: s.UserAgent,
			CreatedAt: s.CreatedAt,
			ExpiresAt: s.ExpiresAt,
			Active:    s.Active,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"sessions": resp})
}

func (s *AuthService) handleRevokeSession(w http.ResponseWriter, r *http.Request) {
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

	var req revokeSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" {
		jsonError(w, "session_id required", http.StatusBadRequest)
		return
	}

	result, err := s.db.Exec(
		"UPDATE user_sessions SET active = false WHERE id = $1 AND user_id = $2 AND active = true",
		req.SessionID, user.ID)
	if err != nil {
		s.logger.Error("Failed to revoke session", "error", err)
		jsonError(w, "failed to revoke session", http.StatusInternalServerError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "revoked"})
}

func (s *AuthService) handleRevokeAllSessions(w http.ResponseWriter, r *http.Request) {
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

	_, err = s.db.Exec(
		"UPDATE user_sessions SET active = false WHERE user_id = $1 AND active = true",
		user.ID)
	if err != nil {
		s.logger.Error("Failed to revoke all sessions", "error", err)
		jsonError(w, "failed to revoke sessions", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "all_sessions_revoked"})
}

func (s *AuthService) adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			jsonError(w, "authorization required", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := common.ValidateJWT(token)
		if err != nil {
			jsonError(w, "invalid token", http.StatusUnauthorized)
			return
		}

		if claims.Role != "admin" {
			jsonError(w, "admin role required", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}

func (s *AuthService) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var users []User
	err := s.db.SelectContext(r.Context(), &users,
		"SELECT id, username, role, active, created_at, updated_at FROM users ORDER BY created_at DESC")
	if err != nil {
		s.logger.Error("Failed to list users", "error", err)
		jsonError(w, "failed to list users", http.StatusInternalServerError)
		return
	}

	resp := make([]UserResponse, len(users))
	for i, u := range users {
		resp[i] = UserResponse{
			ID:        u.ID,
			Username:  u.Username,
			Role:      u.Role,
			Active:    u.Active,
			CreatedAt: u.CreatedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"users": resp})
}

func (s *AuthService) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		jsonError(w, "username and password required", http.StatusBadRequest)
		return
	}

	if req.Role == "" {
		req.Role = "viewer"
	}

	// Validate password policy
	if err := s.config.PasswordPolicy.Validate(req.Password); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		s.logger.Error("Failed to hash password", "error", err)
		jsonError(w, "failed to create user", http.StatusInternalServerError)
		return
	}

	var id string
	err = s.db.QueryRowContext(r.Context(),
		"INSERT INTO users (username, password_hash, role) VALUES ($1, $2, $3) RETURNING id",
		req.Username, string(hash), req.Role).Scan(&id)
	if err != nil {
		s.logger.Error("Failed to create user", "error", err)
		jsonError(w, "failed to create user", http.StatusInternalServerError)
		return
	}

	// Record initial password history
	s.recordPasswordHistory(id, string(hash))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "created"})
}

func (s *AuthService) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := extractIDFromPath(r.URL.Path, "/auth/admin/users/")
	if id == "" {
		jsonError(w, "user id required", http.StatusBadRequest)
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Role != "" {
		_, err := s.db.ExecContext(r.Context(),
			"UPDATE users SET role = $1, updated_at = NOW() WHERE id = $2 AND active = true",
			req.Role, id)
		if err != nil {
			s.logger.Error("Failed to update user", "error", err)
			jsonError(w, "failed to update user", http.StatusInternalServerError)
			return
		}
	}

	if req.Password != "" {
		// Validate password policy
		if err := s.config.PasswordPolicy.Validate(req.Password); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Check password history
		if err := s.checkPasswordHistory(id, req.Password); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			s.logger.Error("Failed to hash password", "error", err)
			jsonError(w, "failed to update user", http.StatusInternalServerError)
			return
		}
		_, err = s.db.ExecContext(r.Context(),
			"UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2 AND active = true",
			string(hash), id)
		if err != nil {
			s.logger.Error("Failed to update password", "error", err)
			jsonError(w, "failed to update user", http.StatusInternalServerError)
			return
		}

		// Record password history
		s.recordPasswordHistory(id, string(hash))
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

func (s *AuthService) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := extractIDFromPath(r.URL.Path, "/auth/admin/users/")
	if id == "" {
		jsonError(w, "user id required", http.StatusBadRequest)
		return
	}

	result, err := s.db.ExecContext(r.Context(),
		"UPDATE users SET active = false, deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND active = true",
		id)
	if err != nil {
		s.logger.Error("Failed to deactivate user", "error", err)
		jsonError(w, "failed to deactivate user", http.StatusInternalServerError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "deactivated"})
}

func extractIDFromPath(path, prefix string) string {
	return strings.TrimPrefix(path, prefix)
}

// Start starts the HTTP server and blocks until ctx is cancelled
func (s *AuthService) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Auth endpoints
	mux.HandleFunc("/auth/login", s.handleLogin)
	mux.HandleFunc("/auth/refresh", s.handleRefreshToken)
	mux.HandleFunc("/auth/password/policy", s.handlePasswordPolicy)
	mux.HandleFunc("/auth/password/change", s.authMiddleware(s.handleChangePassword))

	// Session endpoints
	mux.HandleFunc("/auth/sessions", s.authMiddleware(s.handleListSessions))
	mux.HandleFunc("/auth/sessions/revoke", s.authMiddleware(s.handleRevokeSession))
	mux.HandleFunc("/auth/sessions/revoke-all", s.authMiddleware(s.handleRevokeAllSessions))

	// MFA endpoints
	mux.HandleFunc("/auth/mfa/enroll", s.authMiddleware(s.handleMFAEnroll))
	mux.HandleFunc("/auth/mfa/verify", s.authMiddleware(s.handleMFAVerify))
	mux.HandleFunc("/auth/mfa/status", s.authMiddleware(s.handleMFAStatus))
	mux.HandleFunc("/auth/mfa/recovery", s.handleMFARecovery)

	// SSO endpoints
	mux.HandleFunc("/auth/sso/providers", s.handleSSOProviders)
	mux.HandleFunc("/auth/sso/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/authorize") {
			s.handleSSOAuthorize(w, r)
		} else if strings.HasSuffix(path, "/callback") {
			s.handleSSOCallback(w, r)
		} else if strings.HasSuffix(path, "/acs") {
			s.handleSAMLACS(w, r)
		} else {
			jsonError(w, "not found", http.StatusNotFound)
		}
	})
	mux.HandleFunc("/auth/admin/sso/providers", s.adminOnly(s.handleAdminSSOProviders))

	// API key endpoints
	mux.HandleFunc("/auth/api-keys", s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleListAPIKeys(w, r)
		case http.MethodPost:
			s.handleCreateAPIKey(w, r)
		default:
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/auth/api-keys/", s.authMiddleware(s.handleRevokeAPIKey))

	// Admin user management
	mux.HandleFunc("/auth/admin/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.adminOnly(s.handleAdminListUsers)(w, r)
		case http.MethodPost:
			s.adminOnly(s.handleAdminCreateUser)(w, r)
		default:
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/auth/admin/users/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			s.adminOnly(s.handleAdminUpdateUser)(w, r)
		case http.MethodDelete:
			s.adminOnly(s.handleAdminDeleteUser)(w, r)
		default:
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Health
	handler := common.NewHealthHandler()
	handler.AddDBChecker(s.db.DB, "postgres")
	s.healthHandler = handler
	mux.HandleFunc("/health", handler.Liveness)
	mux.HandleFunc("/ready", handler.Readiness)

	server := &http.Server{
		Addr:         s.config.HTTPAddr,
		Handler:      common.RecoveryMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		s.logger.Info("Auth Service starting", "address", s.config.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Auth server error", "error", err)
		}
	}()

	<-ctx.Done()
	s.logger.Info("Shutting down Auth Service...")
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := common.InitTelemetry("auth"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultAuthConfig()

	if err := config.Validate(); err != nil {
		logger.Error("Invalid configuration", "error", err)
		os.Exit(1)
	}

	common.StartMetricsServer(common.GetEnv("METRICS_ADDR", ":2112"))
	common.StartResourceMonitor(ctx)

	service, err := NewAuthService(ctx, config, logger)
	if err != nil {
		logger.Error("Failed to create auth service", "error", err)
		os.Exit(1)
	}
	defer service.Close()

	if err := service.Start(ctx); err != nil {
		logger.Error("Auth service failed", "error", err)
		os.Exit(1)
	}
}
