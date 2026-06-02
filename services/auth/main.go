package main

import (
	"context"
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
}

// DefaultAuthConfig returns a configuration with sensible defaults
func DefaultAuthConfig() AuthConfig {
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
	TenantID     string     `db:"tenant_id"`
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
	Token string `json:"token"`
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

	token, err := s.authenticateUser(r.Context(), req.Username, req.Password)
	if err != nil {
		s.logger.Warn("Authentication failed", "username", req.Username, "error", err)
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LoginResponse{Token: token})
}

// authenticateUser validates credentials and returns a JWT token
func (s *AuthService) authenticateUser(ctx context.Context, username, password string) (string, error) {
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
			return "", fmt.Errorf("user not found: %w", err)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(localUser.PasswordHash), []byte(password)); err != nil {
			return "", errors.New("invalid password")
		}
		user = &localUser
	}

	return s.generateToken(*user)
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

	// Auto-provision local user if not exists
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
	claims := &common.Claims{
		Username: user.Username,
		Role:     user.Role,
		TenantID: user.TenantID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.config.JWTSecret)
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
	mux.HandleFunc("/auth/login", s.handleLogin)
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
