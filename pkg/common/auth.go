package common

import (
	"context"
	"encoding/base64"
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

var (
	jwtKey    []byte
	jwtKeys   map[string][]byte
	activeKid string
	keyMu     sync.RWMutex
)

func loadJWTKeys() {
	keyMu.Lock()
	defer keyMu.Unlock()

	jwtKey = []byte(os.Getenv("JWT_SECRET"))
	jwtKeys = nil
	activeKid = ""

	keysEnv := os.Getenv("JWT_SECRET_KEYS")
	if keysEnv == "" {
		return
	}

	jwtKeys = make(map[string][]byte)
	for _, pair := range strings.Split(keysEnv, ",") {
		pair = strings.TrimSpace(pair)
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			continue
		}
		kid := strings.TrimSpace(parts[0])
		keyData, err := base64.StdEncoding.DecodeString(strings.TrimSpace(parts[1]))
		if err != nil {
			continue
		}
		jwtKeys[kid] = keyData
	}

	activeKid = os.Getenv("JWT_ACTIVE_KID")
	if activeKid == "" {
		for kid := range jwtKeys {
			activeKid = kid
			break
		}
	}
}

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
	loadJWTKeys()
}

func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func CheckJWTSecret() {
	keyMu.RLock()
	hasKey := len(jwtKey) > 0 || len(jwtKeys) > 0
	keyMu.RUnlock()
	if !hasKey {
		panic("JWT_SECRET or JWT_SECRET_KEYS environment variable is not set")
	}
}

func getJWTKey() []byte {
	keyMu.RLock()
	defer keyMu.RUnlock()
	return jwtKey
}

func ReloadJWTKey() {
	loadJWTKeys()
}

func RotateJWTKey(newKey string) error {
	if newKey == "" {
		return errors.New("JWT key cannot be empty")
	}
	keyMu.Lock()
	defer keyMu.Unlock()

	kid := uuid.New().String()[:8]
	if jwtKeys == nil {
		jwtKeys = make(map[string][]byte)
	}
	jwtKeys[kid] = []byte(newKey)
	activeKid = kid
	return nil
}

type Claims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	TenantID string `json:"tenant_id,omitempty"`
	jwt.RegisteredClaims
}

func SignJWT(claims *Claims) (string, error) {
	keyMu.RLock()
	if jwtKeys != nil {
		kid := activeKid
		key, ok := jwtKeys[kid]
		keyMu.RUnlock()
		if !ok || len(key) == 0 {
			return "", errors.New("no active JWT signing key")
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		token.Header["kid"] = kid
		return token.SignedString(key)
	}

	singleKey := jwtKey
	keyMu.RUnlock()
	if len(singleKey) == 0 {
		return "", errors.New("JWT_SECRET not set")
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(singleKey)
}

func ValidateJWT(tokenString string) (*Claims, error) {
	keyMu.RLock()
	hasMultiKeys := jwtKeys != nil

	var keyFunc jwt.Keyfunc
	if hasMultiKeys {
		keysCopy := make(map[string][]byte, len(jwtKeys))
		for k, v := range jwtKeys {
			keysCopy[k] = v
		}
		keyMu.RUnlock()
		keyFunc = func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			kid, ok := token.Header["kid"].(string)
			if !ok || kid == "" {
				return nil, errors.New("token missing kid header")
			}
			key, ok := keysCopy[kid]
			if !ok {
				return nil, fmt.Errorf("unknown kid: %s", kid)
			}
			return key, nil
		}
	} else {
		singleKey := jwtKey
		keyMu.RUnlock()
		if len(singleKey) == 0 {
			return nil, errors.New("JWT_SECRET not set")
		}
		keyFunc = func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return singleKey, nil
		}
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, keyFunc)

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
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := ValidateJWT(tokenString)
		if err != nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		ctx := r.Context()
		if claims.TenantID != "" {
			ctx = context.WithValue(ctx, TenantKey, claims.TenantID)
		}
		if claims.Username != "" {
			ctx = context.WithValue(ctx, UserKey, claims.Username)
		}
		if claims.Role != "" {
			ctx = context.WithValue(ctx, RoleKey, claims.Role)
		}

		next(w, r.WithContext(ctx))
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
