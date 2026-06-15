package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"golang.org/x/crypto/bcrypt"
)

type PasswordPolicy struct {
	MinLength          int  `json:"min_length"`
	RequireUppercase   bool `json:"require_uppercase"`
	RequireLowercase   bool `json:"require_lowercase"`
	RequireDigit       bool `json:"require_digit"`
	RequireSpecial     bool `json:"require_special"`
	PasswordHistory    int  `json:"password_history"`
	PasswordExpiryDays int  `json:"password_expiry_days"`
	MaxFailedAttempts  int  `json:"max_failed_attempts"`
	LockoutMinutes     int  `json:"lockout_minutes"`
}

func DefaultPasswordPolicy() PasswordPolicy {
	return PasswordPolicy{
		MinLength:          12,
		RequireUppercase:   true,
		RequireLowercase:   true,
		RequireDigit:       true,
		RequireSpecial:     true,
		PasswordHistory:    24,
		PasswordExpiryDays: 90,
		MaxFailedAttempts:  5,
		LockoutMinutes:     30,
	}
}

func (p *PasswordPolicy) Validate(password string) error {
	if len(password) < p.MinLength {
		return fmt.Errorf("password must be at least %d characters", p.MinLength)
	}
	if p.RequireUppercase {
		matched, _ := regexp.MatchString(`[A-Z]`, password)
		if !matched {
			return fmt.Errorf("password must contain an uppercase letter")
		}
	}
	if p.RequireLowercase {
		matched, _ := regexp.MatchString(`[a-z]`, password)
		if !matched {
			return fmt.Errorf("password must contain a lowercase letter")
		}
	}
	if p.RequireDigit {
		matched, _ := regexp.MatchString(`[0-9]`, password)
		if !matched {
			return fmt.Errorf("password must contain a digit")
		}
	}
	if p.RequireSpecial {
		matched, _ := regexp.MatchString(`[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]`, password)
		if !matched {
			return fmt.Errorf("password must contain a special character")
		}
	}
	return nil
}

func (p *PasswordPolicy) GenerateRandomPassword() (string, error) {
	const upper = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const lower = "abcdefghijklmnopqrstuvwxyz"
	const digits = "0123456789"
	const special = "!@#$%^&*()_+-=[]{}|;:,.<>?"

	var charset strings.Builder
	if p.RequireUppercase {
		charset.WriteString(upper)
	}
	if p.RequireLowercase {
		charset.WriteString(lower)
	}
	if p.RequireDigit {
		charset.WriteString(digits)
	}
	if p.RequireSpecial {
		charset.WriteString(special)
	}
	if charset.Len() == 0 {
		charset.WriteString(lower + digits)
	}

	length := p.MinLength
	if length < 12 {
		length = 12
	}
	// Ensure enough room for required char types
	requiredCount := 0
	if p.RequireUppercase {
		requiredCount++
	}
	if p.RequireLowercase {
		requiredCount++
	}
	if p.RequireDigit {
		requiredCount++
	}
	if p.RequireSpecial {
		requiredCount++
	}
	if requiredCount > length {
		length = requiredCount + 4
	}

	var password strings.Builder
	chars := charset.String()
	for i := 0; i < length-requiredCount; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", err
		}
		password.WriteByte(chars[n.Int64()])
	}
	if p.RequireUppercase {
		password.WriteByte(upper[randIndex(len(upper))])
	}
	if p.RequireLowercase {
		password.WriteByte(lower[randIndex(len(lower))])
	}
	if p.RequireDigit {
		password.WriteByte(digits[randIndex(len(digits))])
	}
	if p.RequireSpecial {
		password.WriteByte(special[randIndex(len(special))])
	}

	// Shuffle
	result := []rune(password.String())
	for i := len(result) - 1; i > 0; i-- {
		j, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		result[i], result[j.Int64()] = result[j.Int64()], result[i]
	}

	return string(result), nil
}

func randIndex(max int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}

func (s *AuthService) checkPasswordHistory(userID, newPassword string) error {
	var history []string
	err := s.db.Select(&history,
		`SELECT password_hash FROM password_history
		 WHERE user_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2`,
		userID, s.config.PasswordPolicy.PasswordHistory)
	if err != nil {
		return nil
	}

	for _, hash := range history {
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(newPassword)); err == nil {
			return fmt.Errorf("password has been used recently")
		}
	}
	return nil
}

func (s *AuthService) recordPasswordHistory(userID, passwordHash string) error {
	_, err := s.db.Exec(
		`INSERT INTO password_history (user_id, password_hash) VALUES ($1, $2)`,
		userID, passwordHash)
	if err == nil {
		s.db.Exec(
			`DELETE FROM password_history WHERE user_id = $1 AND id NOT IN (
				SELECT id FROM password_history WHERE user_id = $1
				ORDER BY created_at DESC LIMIT $2
			)`, userID, s.config.PasswordPolicy.PasswordHistory)
	}
	return err
}

func (s *AuthService) isAccountLocked(key string) (bool, time.Duration, error) {
	lockoutMinutes := s.config.PasswordPolicy.LockoutMinutes
	if lockoutMinutes <= 0 {
		lockoutMinutes = 30
	}

	var count int
	err := s.db.Get(&count,
		`SELECT COUNT(*) FROM failed_login_attempts
		 WHERE username = $1
		 AND attempted_at > NOW() - ($2 || ' minutes')::interval`,
		key, fmt.Sprintf("%d", lockoutMinutes))
	if err != nil {
		return false, 0, err
	}

	if count >= s.config.PasswordPolicy.MaxFailedAttempts && s.config.PasswordPolicy.MaxFailedAttempts > 0 {
		var earliest time.Time
		err := s.db.Get(&earliest,
			`SELECT MIN(attempted_at) FROM failed_login_attempts
			 WHERE username = $1
			 AND attempted_at > NOW() - ($2 || ' minutes')::interval`,
			key, fmt.Sprintf("%d", lockoutMinutes))
		if err != nil {
			return true, time.Duration(lockoutMinutes) * time.Minute, nil
		}
		elapsed := time.Since(earliest)
		remaining := time.Duration(lockoutMinutes)*time.Minute - elapsed
		if remaining < 0 {
			return false, 0, nil
		}
		return true, remaining, nil
	}
	return false, 0, nil
}

func (s *AuthService) recordFailedAttempt(key string) {
	s.db.Exec(
		`INSERT INTO failed_login_attempts (username) VALUES ($1)`, key)
}

func (s *AuthService) clearFailedAttempts(key string) {
	s.db.Exec(
		`DELETE FROM failed_login_attempts WHERE username = $1`, key)
}

func (s *AuthService) checkPasswordExpiry(userID string) (bool, error) {
	var passwordChangedAt time.Time
	err := s.db.Get(&passwordChangedAt,
		`SELECT COALESCE(updated_at, created_at) FROM users WHERE id = $1`, userID)
	if err != nil {
		return false, err
	}
	expiryDays := s.config.PasswordPolicy.PasswordExpiryDays
	if expiryDays <= 0 {
		return false, nil
	}
	return time.Since(passwordChangedAt) > time.Duration(expiryDays)*24*time.Hour, nil
}

type passwordChangeRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type passwordChangeResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func (s *AuthService) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
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
		r.Header.Set("X-Username", claims.Username)
		r.Header.Set("X-Role", claims.Role)
		ctx := r.Context()
		ctx = context.WithValue(ctx, common.UserKey, claims.Username)
		ctx = context.WithValue(ctx, common.RoleKey, claims.Role)
		if claims.TenantID != "" {
			ctx = context.WithValue(ctx, common.TenantKey, claims.TenantID)
		}
		next(w, r.WithContext(ctx))
	}
}

func (s *AuthService) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := r.Context().Value(common.UserKey).(string)

	var req passwordChangeRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.NewPassword == "" {
		jsonError(w, "new password required", http.StatusBadRequest)
		return
	}

	// Rate limit password changes
	rateLimitKey := "pwchange:" + username
	if !s.rateLimitPasswordChange(rateLimitKey) {
		jsonError(w, "too many password change attempts", http.StatusTooManyRequests)
		return
	}

	// Verify current password
	var user User
	err := s.db.Get(&user,
		"SELECT id, username, password_hash, role, tenant_id FROM users WHERE username = $1 AND active = true AND deleted_at IS NULL",
		username)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}

	if req.CurrentPassword == "" {
		jsonError(w, "current password is required", http.StatusBadRequest)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		jsonError(w, "current password is incorrect", http.StatusUnauthorized)
		return
	}

	// Validate against password policy
	if err := s.config.PasswordPolicy.Validate(req.NewPassword); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check password history
	if err := s.checkPasswordHistory(user.ID, req.NewPassword); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Hash and save new password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		s.logger.Error("Failed to hash password", "error", err)
		jsonError(w, "failed to update password", http.StatusInternalServerError)
		return
	}

	_, err = s.db.Exec(
		"UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2",
		string(hash), user.ID)
	if err != nil {
		s.logger.Error("Failed to update password", "error", err)
		jsonError(w, "failed to update password", http.StatusInternalServerError)
		return
	}

	// Record password history
	s.recordPasswordHistory(user.ID, string(hash))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(passwordChangeResponse{Status: "ok", Message: "password changed successfully"})
}

func (s *AuthService) rateLimitPasswordChange(key string) bool {
	const maxChanges = 3
	const window = 1 * time.Hour

	var count int
	s.db.Get(&count,
		`SELECT COUNT(*) FROM password_history ph
		 JOIN users u ON ph.user_id = u.id
		 WHERE u.username = $1 AND ph.created_at > NOW() - $2::interval`,
		key[8:], fmt.Sprintf("%d hours", 1))
	return count < maxChanges
}

func (s *AuthService) handlePasswordPolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.config.PasswordPolicy)
}
