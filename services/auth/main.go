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

	"github.com/golang-jwt/jwt/v5"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

// AuthConfig holds configuration for the auth service
type AuthConfig struct {
	HTTPAddr   string
	DBURL      string
	JWTSecret  []byte
	TokenExpiry time.Duration
}

// DefaultAuthConfig returns a configuration with sensible defaults
func DefaultAuthConfig() AuthConfig {
	return AuthConfig{
		HTTPAddr:    ":8081",
		DBURL:       "postgres://dam_admin:dam_password@localhost:5432/dam_vms?sslmode=disable",
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

// Claims represents JWT token claims
type Claims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
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
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.Warn("Invalid login request", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		http.Error(w, "Username and password required", http.StatusBadRequest)
		return
	}

	token, err := s.authenticateUser(r.Context(), req.Username, req.Password)
	if err != nil {
		s.logger.Warn("Authentication failed", "username", req.Username, "error", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
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
	claims := &Claims{
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
	w.WriteHeader(http.StatusOK)
}

// Start starts the HTTP server
func (s *AuthService) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/login", s.handleLogin)
	mux.HandleFunc("/health", s.healthHandler)

	s.logger.Info("Auth Service starting", "address", s.config.HTTPAddr)
	return http.ListenAndServe(s.config.HTTPAddr, mux)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultAuthConfig()
	if addr := os.Getenv("HTTP_ADDR"); addr != "" {
		config.HTTPAddr = addr
	}
	if dbURL := os.Getenv("DB_URL"); dbURL != "" {
		config.DBURL = dbURL
	}

	if err := config.Validate(); err != nil {
		logger.Error("Invalid configuration", "error", err)
		os.Exit(1)
	}

	service, err := NewAuthService(ctx, config, logger)
	if err != nil {
		logger.Error("Failed to create auth service", "error", err)
		os.Exit(1)
	}
	defer service.Close()

	if err := service.Start(); err != nil {
		logger.Error("Auth service failed", "error", err)
		os.Exit(1)
	}
}
