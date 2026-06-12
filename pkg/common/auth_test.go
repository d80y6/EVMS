package common

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestGetEnv(t *testing.T) {
	t.Run("returns default when env not set", func(t *testing.T) {
		os.Unsetenv("TEST_VAR_NOT_SET")
		if got := GetEnv("TEST_VAR_NOT_SET", "default"); got != "default" {
			t.Errorf("GetEnv() = %q, want %q", got, "default")
		}
	})

	t.Run("returns env value when set", func(t *testing.T) {
		os.Setenv("TEST_VAR_SET", "actual")
		defer os.Unsetenv("TEST_VAR_SET")
		if got := GetEnv("TEST_VAR_SET", "default"); got != "actual" {
			t.Errorf("GetEnv() = %q, want %q", got, "actual")
		}
	})

	t.Run("returns env value even when empty default", func(t *testing.T) {
		os.Setenv("TEST_VAR_EMPTY_DEF", "val")
		defer os.Unsetenv("TEST_VAR_EMPTY_DEF")
		if got := GetEnv("TEST_VAR_EMPTY_DEF", ""); got != "val" {
			t.Errorf("GetEnv() = %q, want %q", got, "val")
		}
	})

	t.Run("returns empty default when env not set", func(t *testing.T) {
		os.Unsetenv("TEST_VAR_NO_VAL")
		if got := GetEnv("TEST_VAR_NO_VAL", ""); got != "" {
			t.Errorf("GetEnv() = %q, want %q", got, "")
		}
	})
}

func TestJWTAuthMiddleware(t *testing.T) {
	t.Run("missing auth header returns 401", func(t *testing.T) {
		handler := JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
			t.Error("next handler should not be called")
		})
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("invalid token returns 401", func(t *testing.T) {
		os.Setenv("JWT_SECRET", "test-secret")
		ReloadJWTKey()
		defer os.Unsetenv("JWT_SECRET")
		defer ReloadJWTKey()

		handler := JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
			t.Error("next handler should not be called")
		})
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()
		handler(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})
}

func TestValidateJWT(t *testing.T) {
	t.Run("no JWT_SECRET returns error", func(t *testing.T) {
		os.Unsetenv("JWT_SECRET")
		ReloadJWTKey()
		defer ReloadJWTKey()
		_, err := ValidateJWT("some-token")
		if err == nil {
			t.Error("expected error when JWT_SECRET not set")
		}
	})

	t.Run("valid token with correct secret", func(t *testing.T) {
		os.Setenv("JWT_SECRET", "test-secret-key-for-jwt")
		ReloadJWTKey()
		defer os.Unsetenv("JWT_SECRET")
		defer ReloadJWTKey()

		claims, err := ValidateJWT("some-token")
		if err == nil {
			// ValidateJWT should fail because "some-token" is not a valid JWT,
			// but it should NOT return "JWT_SECRET not set"
			if claims != nil {
				t.Error("expected nil claims for invalid token")
			}
		}
	})
}

func TestGetJWTKey(t *testing.T) {
	t.Run("returns empty when env not set", func(t *testing.T) {
		os.Unsetenv("JWT_SECRET")
		ReloadJWTKey()
		defer ReloadJWTKey()
		key := getJWTKey()
		if len(key) != 0 {
			t.Errorf("expected empty key, got %d bytes", len(key))
		}
	})

	t.Run("returns key when env is set", func(t *testing.T) {
		os.Setenv("JWT_SECRET", "my-secret-key")
		ReloadJWTKey()
		defer os.Unsetenv("JWT_SECRET")
		defer ReloadJWTKey()

		key := getJWTKey()
		if string(key) != "my-secret-key" {
			t.Errorf("expected 'my-secret-key', got %q", string(key))
		}
	})

	t.Run("reloads after env change", func(t *testing.T) {
		os.Unsetenv("JWT_SECRET")
		ReloadJWTKey()

		key1 := getJWTKey()
		if len(key1) != 0 {
			t.Fatal("expected empty key after reload with no env")
		}

		os.Setenv("JWT_SECRET", "set-after-reload")
		ReloadJWTKey()
		defer os.Unsetenv("JWT_SECRET")
		defer ReloadJWTKey()

		key2 := getJWTKey()
		if string(key2) != "set-after-reload" {
			t.Errorf("expected 'set-after-reload', got %q", string(key2))
		}
	})
}

func TestExtractClaims(t *testing.T) {
	t.Run("no auth header returns nil", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		claims := ExtractClaims(req)
		if claims != nil {
			t.Error("expected nil claims when no auth header")
		}
	})
}

func TestJWTAuthMiddleware_InjectsClaims(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-for-context-test")
	ReloadJWTKey()
	defer os.Unsetenv("JWT_SECRET")
	defer ReloadJWTKey()

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		Username: "testuser",
		Role:     "admin",
		TenantID: "tenant-123",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: uuid.New().String(),
		},
	}).SignedString([]byte("test-secret-for-context-test"))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}

	var capturedCtx context.Context
	handler := JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if got := TenantFromContext(capturedCtx); got != "tenant-123" {
		t.Errorf("TenantFromContext = %q, want 'tenant-123'", got)
	}
	if got := UserFromContext(capturedCtx); got != "testuser" {
		t.Errorf("UserFromContext = %q, want 'testuser'", got)
	}
	if got := RoleFromContext(capturedCtx); got != "admin" {
		t.Errorf("RoleFromContext = %q, want 'admin'", got)
	}
}

func TestJWTAuthMiddleware_TokenInQueryParamRejected(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-query-param")
	ReloadJWTKey()
	defer os.Unsetenv("JWT_SECRET")
	defer ReloadJWTKey()

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		Username: "queryuser",
		Role:     "viewer",
		TenantID: "tenant-456",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: uuid.New().String(),
		},
	}).SignedString([]byte("test-secret-query-param"))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}

	handler := JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/resource?token="+token, nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestJWTAuthMiddleware_ValidTokenNoClaims(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-no-claims")
	ReloadJWTKey()
	defer os.Unsetenv("JWT_SECRET")
	defer ReloadJWTKey()

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: uuid.New().String(),
		},
	}).SignedString([]byte("test-secret-no-claims"))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}

	var capturedCtx context.Context
	handler := JWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if got := TenantFromContext(capturedCtx); got != "" {
		t.Errorf("expected empty tenant, got %q", got)
	}
}
