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
	"syscall"
	"time"

	"github.com/dam-vms/dam/pkg/common"
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
}

// DefaultAuthConfig returns a configuration with sensible defaults
func DefaultAuthConfig() AuthConfig {
	return AuthConfig{
		HTTPAddr:    ":8081",
		DBURL:       common.GetEnv("DB_URL", "postgres://dam_admin:dam_password@localhost:5432/dam_vms?sslmode=disable"),
		JWTSecret:   []byte(os.Getenv("JWT_SECRET")),
		TokenExpiry: 24 * time.Hour,
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
	ID           string    `db:"id"`
	Username     string    `db:"username"`
	PasswordHash string    `db:"password_hash"`
	Role         string    `db:"role"`
	CreatedAt    time.Time `db:"created_at"`
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
	logger *slog.Logger
	db     *sqlx.DB
	config AuthConfig
}

// NewAuthService creates a new auth service instance
func NewAuthService(ctx context.Context, config AuthConfig, logger *slog.Logger) (*AuthService, error) {
	db, err := sqlx.ConnectContext(ctx, "postgres", config.DBURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
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
	var user User
	err := s.db.GetContext(ctx, &user,
		"SELECT id, username, password_hash, role FROM users WHERE username = $1",
		username)
	if err != nil {
		return "", fmt.Errorf("user not found: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", errors.New("invalid password")
	}

	return s.generateToken(user)
}

// generateToken creates a JWT token for a user
func (s *AuthService) generateToken(user User) (string, error) {
	expirationTime := time.Now().Add(s.config.TokenExpiry)
	claims := &common.Claims{
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.config.JWTSecret)
}

// healthHandler handles health check requests
func (s *AuthService) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// Start starts the HTTP server and blocks until ctx is cancelled
func (s *AuthService) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/login", s.handleLogin)
	mux.HandleFunc("/health", s.healthHandler)

	server := &http.Server{
		Addr:         s.config.HTTPAddr,
		Handler:      mux,
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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultAuthConfig()

	if err := config.Validate(); err != nil {
		logger.Error("Invalid configuration", "error", err)
		os.Exit(1)
	}

	common.StartMetricsServer(common.GetEnv("METRICS_ADDR", ":2112"))

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
