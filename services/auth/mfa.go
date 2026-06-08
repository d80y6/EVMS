package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"golang.org/x/crypto/bcrypt"
)

type MFASettings struct {
	ID        string    `db:"id"`
	UserID    string    `db:"user_id"`
	Secret    string    `db:"secret"`
	Enabled   bool      `db:"enabled"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type MFARecoveryCode struct {
	ID        string     `db:"id"`
	UserID    string     `db:"user_id"`
	CodeHash  string     `db:"code_hash"`
	Used      bool       `db:"used"`
	UsedAt    *time.Time `db:"used_at"`
	CreatedAt time.Time  `db:"created_at"`
}

type mfaEnrollRequest struct{}
type mfaEnrollResponse struct {
	Secret      string   `json:"secret"`
	URI         string   `json:"uri"`
	RecoveryCodes []string `json:"recovery_codes"`
}

type mfaVerifyRequest struct {
	Code string `json:"code"`
}

type mfaStatusResponse struct {
	Enabled bool `json:"enabled"`
}

type mfaRecoveryRequest struct {
	Code string `json:"code"`
}

func generateTOTPSecret() (string, error) {
	secret := make([]byte, 20)
	_, err := rand.Read(secret)
	if err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret), nil
}

func generateTOTPCode(secret string, timestamp time.Time) (string, error) {
	counter := uint64(timestamp.Unix()) / 30
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return "", err
	}

	counterBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(counterBytes, counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(counterBytes)
	hash := mac.Sum(nil)

	offset := hash[len(hash)-1] & 0x0f
	truncated := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff
	code := truncated % 1000000

	return fmt.Sprintf("%06d", code), nil
}

func validateTOTPCode(secret, code string) bool {
	now := time.Now()
	for i := -1; i <= 1; i++ {
		t := now.Add(time.Duration(i) * 30 * time.Second)
		expected, err := generateTOTPCode(secret, t)
		if err == nil && subtle.ConstantTimeCompare([]byte(code), []byte(expected)) == 1 {
			return true
		}
	}
	return false
}

func generateRecoveryCodes(count int) ([]string, []string, error) {
	codes := make([]string, count)
	hashed := make([]string, count)
	for i := 0; i < count; i++ {
		codeBytes := make([]byte, 10)
		if _, err := rand.Read(codeBytes); err != nil {
			return nil, nil, err
		}
		code := hex.EncodeToString(codeBytes)
		code = code[:12]
		// Format as XXXXX-XXXXX
		formatted := code[:5] + "-" + code[5:]
		codes[i] = formatted

		hash, err := bcrypt.GenerateFromPassword([]byte(formatted), bcrypt.DefaultCost)
		if err != nil {
			return nil, nil, err
		}
		hashed[i] = string(hash)
	}
	return codes, hashed, nil
}

func (s *AuthService) handleMFAEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := r.Context().Value(common.UserKey).(string)

	var user User
	err := s.db.Get(&user,
		"SELECT id, username FROM users WHERE username = $1 AND active = true AND deleted_at IS NULL",
		username)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}

	// Check if already enrolled
	var existing MFASettings
	err = s.db.Get(&existing, "SELECT id FROM mfa_settings WHERE user_id = $1", user.ID)
	if err == nil && existing.ID != "" {
		// Regenerate secret
		secret, err := generateTOTPSecret()
		if err != nil {
			jsonError(w, "failed to generate secret", http.StatusInternalServerError)
			return
		}
		_, err = s.db.Exec("UPDATE mfa_settings SET secret = $1, enabled = false, updated_at = NOW() WHERE user_id = $2",
			secret, user.ID)
		if err != nil {
			jsonError(w, "failed to update MFA settings", http.StatusInternalServerError)
			return
		}

		// Remove old recovery codes
		s.db.Exec("DELETE FROM mfa_recovery_codes WHERE user_id = $1", user.ID)

		// Generate new recovery codes
		plainCodes, hashedCodes, err := generateRecoveryCodes(8)
		if err != nil {
			jsonError(w, "failed to generate recovery codes", http.StatusInternalServerError)
			return
		}

		for _, h := range hashedCodes {
			s.db.Exec("INSERT INTO mfa_recovery_codes (user_id, code_hash) VALUES ($1, $2)", user.ID, h)
		}

		uri := fmt.Sprintf("otpauth://totp/EVMS:%s?secret=%s&issuer=EVMS&algorithm=SHA1&digits=6&period=30",
			username, secret)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mfaEnrollResponse{
			Secret:        secret,
			URI:           uri,
			RecoveryCodes: plainCodes,
		})
		return
	}

	secret, err := generateTOTPSecret()
	if err != nil {
		jsonError(w, "failed to generate secret", http.StatusInternalServerError)
		return
	}

	_, err = s.db.Exec(
		"INSERT INTO mfa_settings (user_id, secret, enabled) VALUES ($1, $2, false)",
		user.ID, secret)
	if err != nil {
		jsonError(w, "failed to save MFA settings", http.StatusInternalServerError)
		return
	}

	plainCodes, hashedCodes, err := generateRecoveryCodes(8)
	if err != nil {
		jsonError(w, "failed to generate recovery codes", http.StatusInternalServerError)
		return
	}

	for _, h := range hashedCodes {
		s.db.Exec("INSERT INTO mfa_recovery_codes (user_id, code_hash) VALUES ($1, $2)", user.ID, h)
	}

	uri := fmt.Sprintf("otpauth://totp/EVMS:%s?secret=%s&issuer=EVMS&algorithm=SHA1&digits=6&period=30",
		username, secret)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mfaEnrollResponse{
		Secret:        secret,
		URI:           uri,
		RecoveryCodes: plainCodes,
	})
}

func (s *AuthService) handleMFAVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := r.Context().Value(common.UserKey).(string)

	var req mfaVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Code == "" {
		jsonError(w, "verification code required", http.StatusBadRequest)
		return
	}

	var user User
	err := s.db.Get(&user,
		"SELECT id, username, password_hash, role, tenant_id FROM users WHERE username = $1 AND active = true AND deleted_at IS NULL",
		username)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}

	var mfa MFASettings
	err = s.db.Get(&mfa, "SELECT secret, enabled FROM mfa_settings WHERE user_id = $1", user.ID)
	if err != nil {
		jsonError(w, "MFA not configured", http.StatusBadRequest)
		return
	}

	if !mfa.Enabled {
		// First-time verification - enable MFA
		if !validateTOTPCode(mfa.Secret, req.Code) {
			jsonError(w, "invalid verification code", http.StatusUnauthorized)
			return
		}
		_, err = s.db.Exec("UPDATE mfa_settings SET enabled = true, updated_at = NOW() WHERE user_id = $1", user.ID)
		if err != nil {
			jsonError(w, "failed to enable MFA", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "enabled"})
		return
	}

	// Verify TOTP for login
	if !validateTOTPCode(mfa.Secret, req.Code) {
		jsonError(w, "invalid verification code", http.StatusUnauthorized)
		return
	}

	// Generate final JWT
	token, err := s.generateToken(user)
	if err != nil {
		jsonError(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LoginResponse{Token: token})
}

func (s *AuthService) handleMFAStatus(w http.ResponseWriter, r *http.Request) {
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

	var mfa MFASettings
	err = s.db.Get(&mfa, "SELECT enabled FROM mfa_settings WHERE user_id = $1", user.ID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mfaStatusResponse{Enabled: false})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mfaStatusResponse{Enabled: mfa.Enabled})
}

func (s *AuthService) handleMFARecovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req mfaRecoveryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Code == "" {
		jsonError(w, "recovery code required", http.StatusBadRequest)
		return
	}

	// Username should come from the temporary MFA token
	username := r.Context().Value(common.UserKey).(string)
	if username == "" {
		// Try to get from a temporary token
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := common.ValidateJWT(tokenStr)
			if err == nil {
				username = claims.Username
			}
		}
	}

	if username == "" {
		jsonError(w, "authentication required", http.StatusUnauthorized)
		return
	}

	var user User
	err := s.db.Get(&user,
		"SELECT id FROM users WHERE username = $1 AND active = true AND deleted_at IS NULL",
		username)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}

	var codes []MFARecoveryCode
	err = s.db.Select(&codes,
		"SELECT id, code_hash, used FROM mfa_recovery_codes WHERE user_id = $1 AND used = false",
		user.ID)
	if err != nil || len(codes) == 0 {
		jsonError(w, "no recovery codes available", http.StatusBadRequest)
		return
	}

	for _, c := range codes {
		if bcrypt.CompareHashAndPassword([]byte(c.CodeHash), []byte(req.Code)) == nil {
			s.db.Exec("UPDATE mfa_recovery_codes SET used = true, used_at = NOW() WHERE id = $1", c.ID)

			// Update MFA as not disabled - but user can re-enable
			s.db.Exec("UPDATE mfa_settings SET enabled = false, updated_at = NOW() WHERE user_id = $1", user.ID)

			// Generate a new temporary token for MFA re-enrollment
			tempToken, err := s.generateMFAToken(user)
			if err != nil {
				jsonError(w, "failed to generate token", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"status":     "recovered",
				"mfa_token":  tempToken,
				"message":    "MFA has been disabled. Please re-enroll and verify a new authenticator.",
			})
			return
		}
	}

	jsonError(w, "invalid recovery code", http.StatusUnauthorized)
}

func (s *AuthService) generateMFAToken(user User) (string, error) {
	return s.generateToken(user)
}
