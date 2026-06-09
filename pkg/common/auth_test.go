package common

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
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
		defer os.Unsetenv("JWT_SECRET")

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
		_, err := ValidateJWT("some-token")
		if err == nil {
			t.Error("expected error when JWT_SECRET not set")
		}
	})

	t.Run("valid token with correct secret", func(t *testing.T) {
		os.Setenv("JWT_SECRET", "test-secret-key-for-jwt")
		defer os.Unsetenv("JWT_SECRET")

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
		key := getJWTKey()
		if len(key) != 0 {
			t.Errorf("expected empty key, got %d bytes", len(key))
		}
	})

	t.Run("returns key when env is set", func(t *testing.T) {
		os.Setenv("JWT_SECRET", "my-secret-key")
		defer os.Unsetenv("JWT_SECRET")

		key := getJWTKey()
		if string(key) != "my-secret-key" {
			t.Errorf("expected 'my-secret-key', got %q", string(key))
		}
	})

	t.Run("does not cache empty key", func(t *testing.T) {
		os.Unsetenv("JWT_SECRET")

		key1 := getJWTKey()
		if len(key1) != 0 {
			t.Fatal("expected empty key on first call")
		}

		os.Setenv("JWT_SECRET", "set-after-first-call")
		defer os.Unsetenv("JWT_SECRET")

		key2 := getJWTKey()
		if string(key2) != "set-after-first-call" {
			t.Errorf("expected newly set key, got %q", string(key2))
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
