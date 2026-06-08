package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type SystemConfig struct {
	Key         string          `db:"key" json:"key"`
	Value       json.RawMessage `db:"value" json:"value"`
	Category    string          `db:"category" json:"category"`
	Description string          `db:"description" json:"description"`
	Schema      json.RawMessage `db:"schema_json" json:"schema_json"`
	Version     int             `db:"version" json:"version"`
	CreatedAt   time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time       `db:"updated_at" json:"updated_at"`
}

type ConfigHistory struct {
	ID        string          `db:"id" json:"id"`
	ConfigKey string          `db:"config_key" json:"config_key"`
	OldValue  json.RawMessage `db:"old_value" json:"old_value"`
	NewValue  json.RawMessage `db:"new_value" json:"new_value"`
	ChangedBy string          `db:"changed_by" json:"changed_by"`
	ChangedAt time.Time       `db:"changed_at" json:"changed_at"`
}

var validCategories = map[string]bool{
	"general":       true,
	"retention":     true,
	"security":      true,
	"notifications": true,
	"storage":       true,
	"ai":            true,
}

var defaultConfigs = map[string]SystemConfig{
	"retention.days": {
		Key: "retention.days", Category: "retention",
		Description: "Default recording retention in days",
		Value: json.RawMessage(`30`),
		Schema: json.RawMessage(`{"type":"integer","minimum":1,"maximum":365}`),
	},
	"security.password_min_length": {
		Key: "security.password_min_length", Category: "security",
		Description: "Minimum password length",
		Value: json.RawMessage(`8`),
		Schema: json.RawMessage(`{"type":"integer","minimum":6,"maximum":128}`),
	},
	"security.max_login_attempts": {
		Key: "security.max_login_attempts", Category: "security",
		Description: "Maximum failed login attempts before lockout",
		Value: json.RawMessage(`5`),
		Schema: json.RawMessage(`{"type":"integer","minimum":1,"maximum":100}`),
	},
	"security.session_timeout_minutes": {
		Key: "security.session_timeout_minutes", Category: "security",
		Description: "Session timeout in minutes",
		Value: json.RawMessage(`60`),
		Schema: json.RawMessage(`{"type":"integer","minimum":5,"maximum":1440}`),
	},
	"notifications.email_from": {
		Key: "notifications.email_from", Category: "notifications",
		Description: "Default from address for email notifications",
		Value: json.RawMessage(`"noreply@evms.local"`),
		Schema: json.RawMessage(`{"type":"string","format":"email"}`),
	},
	"notifications.rate_limit_per_min": {
		Key: "notifications.rate_limit_per_min", Category: "notifications",
		Description: "Default rate limit per minute for notifications",
		Value: json.RawMessage(`60`),
		Schema: json.RawMessage(`{"type":"integer","minimum":1,"maximum":10000}`),
	},
	"storage.recording_path": {
		Key: "storage.recording_path", Category: "storage",
		Description: "Base path for recording storage",
		Value: json.RawMessage(`"/recordings"`),
		Schema: json.RawMessage(`{"type":"string"}`),
	},
	"storage.export_path": {
		Key: "storage.export_path", Category: "storage",
		Description: "Base path for export storage",
		Value: json.RawMessage(`"/exports"`),
		Schema: json.RawMessage(`{"type":"string"}`),
	},
	"ai.detection_confidence_threshold": {
		Key: "ai.detection_confidence_threshold", Category: "ai",
		Description: "Minimum confidence threshold for AI detections",
		Value: json.RawMessage(`0.5`),
		Schema: json.RawMessage(`{"type":"number","minimum":0,"maximum":1}`),
	},
	"ai.face_recognition_enabled": {
		Key: "ai.face_recognition_enabled", Category: "ai",
		Description: "Enable face recognition",
		Value: json.RawMessage(`false`),
		Schema: json.RawMessage(`{"type":"boolean"}`),
	},
	"general.site_name": {
		Key: "general.site_name", Category: "general",
		Description: "Display name for the VMS installation",
		Value: json.RawMessage(`"EVMS"`),
		Schema: json.RawMessage(`{"type":"string","minLength":1,"maxLength":100}`),
	},
}

type ConfigManager struct {
	db *sqlx.DB
}

func NewConfigManager(db *sqlx.DB) *ConfigManager {
	return &ConfigManager{db: db}
}

func (m *ConfigManager) SeedDefaults() error {
	if m.db == nil {
		return nil
	}
	for _, cfg := range defaultConfigs {
		var count int
		if err := m.db.Get(&count, "SELECT COUNT(*) FROM system_config WHERE key = $1", cfg.Key); err != nil {
			return err
		}
		if count == 0 {
			now := time.Now().UTC()
			if _, err := m.db.Exec(
				`INSERT INTO system_config (key, value, category, description, schema_json, version, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, 1, $6, $6)`,
				cfg.Key, cfg.Value, cfg.Category, cfg.Description, cfg.Schema, now,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func handleAdminConfig(db *sqlx.DB, logger *ConfigManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			listAllConfig(db, w, r)
		case http.MethodPut:
			updateConfig(db, w, r, "")
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleAdminConfigCategory(db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
			return
		}
		category := strings.TrimPrefix(r.URL.Path, "/api/admin/config/")
		if category == "" {
			http.Error(w, "category required", http.StatusBadRequest)
			return
		}
		if !validCategories[category] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid category: " + category})
			return
		}
		switch r.Method {
		case http.MethodGet:
			listConfigByCategory(db, w, r, category)
		case http.MethodPut:
			updateConfig(db, w, r, category)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleConfigHistory(db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		q := r.URL.Query()
		configKey := q.Get("key")

		var history []ConfigHistory
		var err error
		if configKey != "" {
			err = db.Select(&history, "SELECT id, config_key, old_value, new_value, changed_by, changed_at FROM system_config_history WHERE config_key = $1 ORDER BY changed_at DESC LIMIT 100", configKey)
		} else {
			err = db.Select(&history, "SELECT id, config_key, old_value, new_value, changed_by, changed_at FROM system_config_history ORDER BY changed_at DESC LIMIT 100")
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query config history"})
			return
		}
		if history == nil {
			history = []ConfigHistory{}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"history": history})
	}
}

func handleConfigExport(db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var configs []SystemConfig
		if err := db.Select(&configs, "SELECT key, value, category, description, schema_json, version, created_at, updated_at FROM system_config ORDER BY key"); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to export config"})
			return
		}

		q := r.URL.Query()
		format := q.Get("format")
		if format == "yaml" {
			out, err := json.Marshal(configs)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to marshal config"})
				return
			}
			w.Header().Set("Content-Type", "application/x-yaml")
			w.Write(out)
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{"config": configs})
	}
}

func handleConfigImport(db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database not configured"})
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Config []SystemConfig `json:"config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		actor := r.Header.Get("X-Username")
		if actor == "" {
			actor = "system"
		}

		imported := 0
		for _, cfg := range req.Config {
			if cfg.Key == "" || !validCategories[cfg.Category] {
				continue
			}

			var existingValue json.RawMessage
			var existingVersion int
			err := db.Get(&existingValue, "SELECT value FROM system_config WHERE key = $1", cfg.Key)
			if err != nil {
				continue
			}
			if err := db.Get(&existingVersion, "SELECT version FROM system_config WHERE key = $1", cfg.Key); err != nil {
				existingVersion = 0
			}

			tx, err := db.Beginx()
			if err != nil {
				continue
			}

			if _, err := tx.Exec(
				`INSERT INTO system_config_history (id, config_key, old_value, new_value, changed_by, changed_at) VALUES ($1, $2, $3, $4, $5, NOW())`,
				uuid.New().String(), cfg.Key, existingValue, cfg.Value, actor,
			); err != nil {
				tx.Rollback()
				continue
			}

			if _, err := tx.Exec(
				`UPDATE system_config SET value = $1, version = $2, updated_at = NOW() WHERE key = $3`,
				cfg.Value, existingVersion+1, cfg.Key,
			); err != nil {
				tx.Rollback()
				continue
			}

			if err := tx.Commit(); err != nil {
				continue
			}
			imported++
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "imported", "count": imported})
	}
}

func listAllConfig(db *sqlx.DB, w http.ResponseWriter, r *http.Request) {
	var configs []SystemConfig
	if err := db.Select(&configs, "SELECT key, value, category, description, schema_json, version, created_at, updated_at FROM system_config ORDER BY category, key"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list config"})
		return
	}
	if configs == nil {
		configs = []SystemConfig{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"config": configs})
}

func listConfigByCategory(db *sqlx.DB, w http.ResponseWriter, r *http.Request, category string) {
	var configs []SystemConfig
	if err := db.Select(&configs, "SELECT key, value, category, description, schema_json, version, created_at, updated_at FROM system_config WHERE category = $1 ORDER BY key", category); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list config"})
		return
	}
	if configs == nil {
		configs = []SystemConfig{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"config": configs})
}

func updateConfig(db *sqlx.DB, w http.ResponseWriter, r *http.Request, category string) {
	var req map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	actor := r.Header.Get("X-Username")
	if actor == "" {
		actor = "system"
	}

	updated := 0
	for key, newValue := range req {
		if category != "" {
			var cat string
			if err := db.Get(&cat, "SELECT category FROM system_config WHERE key = $1", key); err != nil {
				continue
			}
			if cat != category {
				continue
			}
		}

		var existing struct {
			Value   json.RawMessage `db:"value"`
			Version int             `db:"version"`
		}
		if err := db.Get(&existing, "SELECT value, version FROM system_config WHERE key = $1", key); err != nil {
			continue
		}

		tx, err := db.Beginx()
		if err != nil {
			continue
		}

		if _, err := tx.Exec(
			`INSERT INTO system_config_history (id, config_key, old_value, new_value, changed_by, changed_at) VALUES ($1, $2, $3, $4, $5, NOW())`,
			uuid.New().String(), key, existing.Value, newValue, actor,
		); err != nil {
			tx.Rollback()
			continue
		}

		if _, err := tx.Exec(
			`UPDATE system_config SET value = $1, version = $2, updated_at = NOW() WHERE key = $3`,
			newValue, existing.Version+1, key,
		); err != nil {
			tx.Rollback()
			continue
		}

		if err := tx.Commit(); err != nil {
			continue
		}
		updated++
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "updated", "count": updated})
}
