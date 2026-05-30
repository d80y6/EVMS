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
}
