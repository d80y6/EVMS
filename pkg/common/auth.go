package common

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"google.golang.org/grpc/encoding"
	_ "google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/metadata"
)

type contextKey string

const (
	TenantKey contextKey = "tenant_id"
	UserKey   contextKey = "username"
	RoleKey   contextKey = "role"
	UserIDKey contextKey = "user_id"
)

func TenantFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(TenantKey).(string); ok {
		return v
	}
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if t := md.Get("tenant_id"); len(t) > 0 {
			return t[0]
		}
	}
	return ""
}

func UserFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(UserKey).(string); ok {
		return v
	}
	return ""
}

func RoleFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(RoleKey).(string); ok {
		return v
	}
	return ""
}

func GetUserIDFromContext(ctx context.Context) (uuid.UUID, error) {
	if v, ok := ctx.Value(UserIDKey).(uuid.UUID); ok {
		return v, nil
	}
	return uuid.Nil, errors.New("user id not found in context")
}

var (
	jwtKey     []byte
	jwtKeyOnce sync.Once
)

// GetEnv retrieves an environment variable with a default value fallback.
type JSONCodec struct{}

func (c JSONCodec) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (c JSONCodec) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func (c JSONCodec) Name() string { return "json" }

func NewJSONCodec() *JSONCodec { return &JSONCodec{} }

func init() {
	encoding.RegisterCodec(NewJSONCodec())
}

func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getJWTKey() []byte {
	jwtKeyOnce.Do(func() {
		if key := os.Getenv("JWT_SECRET"); key != "" {
			jwtKey = []byte(key)
		}
	})
	return jwtKey
}

type Claims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	TenantID string `json:"tenant_id,omitempty"`
	jwt.RegisteredClaims
}

func ValidateJWT(tokenString string) (*Claims, error) {
	key := getJWTKey()
	if len(key) == 0 {
		return nil, errors.New("JWT_SECRET not set")
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return getJWTKey(), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

func JWTAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		authHeader = r.URL.Query().Get("token")
		if authHeader != "" {
			r.URL.RawQuery = ""
		}
	}

		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		_, err := ValidateJWT(tokenString)
		if err != nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

func MustNewReverseProxy(envKey, defaultURL string) *httputil.ReverseProxy {
	targetURL := GetEnv(envKey, defaultURL)
	u, err := url.Parse(targetURL)
	if err != nil {
		panic(fmt.Sprintf("MustNewReverseProxy: failed to parse URL %s: %v", targetURL, err))
	}
	return httputil.NewSingleHostReverseProxy(u)
}

func ExtractClaims(r *http.Request) *Claims {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil
	}
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	claims, err := ValidateJWT(tokenString)
	if err != nil {
		return nil
	}
	return claims
}
