package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/jmoiron/sqlx"
)

type Condition struct {
	Source   string `json:"source"`
	CameraID string `json:"camera_id"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
	Schedule string `json:"schedule"`
}

type Action struct {
	Type   string            `json:"type"`
	Target string            `json:"target"`
	Params map[string]string `json:"params"`
}

type Rule struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Enabled    bool        `json:"enabled"`
	Conditions []Condition `json:"conditions"`
	Actions    []Action    `json:"actions"`
	Logic      string      `json:"logic"`
}

type RuleEngine struct {
	mu     sync.RWMutex
	rules  map[string]*Rule
	db     *sqlx.DB
	logger *slog.Logger
}

func NewRuleEngine(db *sqlx.DB, logger *slog.Logger) *RuleEngine {
	re := &RuleEngine{
		rules:  make(map[string]*Rule),
		db:     db,
		logger: logger,
	}
	re.loadFromDB()
	return re
}

func (re *RuleEngine) loadFromDB() {
	if re.db == nil {
		return
	}
	var rows []struct {
		ID         string          `db:"id"`
		Name       string          `db:"name"`
		Enabled    bool            `db:"enabled"`
		Logic      string          `db:"logic"`
		Conditions json.RawMessage `db:"conditions"`
		Actions    json.RawMessage `db:"actions"`
	}
	if err := re.db.Select(&rows, "SELECT id, name, enabled, logic, conditions, actions FROM rules"); err != nil {
		re.logger.Warn("Failed to load rules from DB", "error", err)
		return
	}
	for _, row := range rows {
		var conditions []Condition
		var actions []Action
		json.Unmarshal(row.Conditions, &conditions)
		json.Unmarshal(row.Actions, &actions)
		re.rules[row.ID] = &Rule{
			ID:         row.ID,
			Name:       row.Name,
			Enabled:    row.Enabled,
			Logic:      row.Logic,
			Conditions: conditions,
			Actions:    actions,
		}
	}
	re.logger.Info("Loaded rules from database", "count", len(rows))
}

func (re *RuleEngine) saveRule(rule *Rule) error {
	if re.db == nil {
		return nil
	}
	conds, _ := json.Marshal(rule.Conditions)
	acts, _ := json.Marshal(rule.Actions)
	_, err := re.db.Exec(`INSERT INTO rules (id, name, enabled, logic, conditions, actions, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE SET name=$2, enabled=$3, logic=$4, conditions=$5, actions=$6, updated_at=NOW()`,
		rule.ID, rule.Name, rule.Enabled, rule.Logic, conds, acts)
	return err
}

func (re *RuleEngine) deleteRule(id string) error {
	if re.db == nil {
		return nil
	}
	_, err := re.db.Exec("DELETE FROM rules WHERE id=$1", id)
	return err
}

func (re *RuleEngine) AddRule(rule *Rule) {
	re.mu.Lock()
	defer re.mu.Unlock()
	re.rules[rule.ID] = rule
	if err := re.saveRule(rule); err != nil {
		re.logger.Error("Failed to persist rule", "id", rule.ID, "error", err)
	}
}

func (re *RuleEngine) RemoveRule(id string) {
	re.mu.Lock()
	defer re.mu.Unlock()
	delete(re.rules, id)
	if err := re.deleteRule(id); err != nil {
		re.logger.Error("Failed to remove rule from DB", "id", id, "error", err)
	}
}

func (re *RuleEngine) Evaluate(event map[string]interface{}) []Action {
	re.mu.RLock()
	defer re.mu.RUnlock()

	var triggered []Action
	for _, rule := range re.rules {
		if !rule.Enabled {
			continue
		}
		if re.matches(rule, event) {
			triggered = append(triggered, rule.Actions...)
		}
	}
	return triggered
}

func (re *RuleEngine) matches(rule *Rule, event map[string]interface{}) bool {
	if len(rule.Conditions) == 0 {
		return true
	}

	results := make([]bool, len(rule.Conditions))
	for i, cond := range rule.Conditions {
		results[i] = re.evaluateCondition(cond, event)
	}

	if rule.Logic == "OR" {
		for _, r := range results {
			if r {
				return true
			}
		}
		return false
	}
	for _, r := range results {
		if !r {
			return false
		}
	}
	return true
}

func (re *RuleEngine) evaluateCondition(cond Condition, event map[string]interface{}) bool {
	if cond.CameraID != "" && cond.CameraID != event["camera_id"] {
		return false
	}

	sourceVal, ok := event[cond.Source]
	if !ok {
		return false
	}

	valStr := fmt.Sprintf("%v", sourceVal)
	switch cond.Operator {
	case "equals":
		return valStr == cond.Value
	case "contains":
		return strings.Contains(valStr, cond.Value)
	case "gt":
		var v, t float64
		fmt.Sscanf(valStr, "%f", &v)
		fmt.Sscanf(cond.Value, "%f", &t)
		return v > t
	case "lt":
		var v, t float64
		fmt.Sscanf(valStr, "%f", &v)
		fmt.Sscanf(cond.Value, "%f", &t)
		return v < t
	}
	return false
}

func (re *RuleEngine) HandleHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		re.mu.RLock()
		rules := make([]*Rule, 0, len(re.rules))
		for _, rl := range re.rules {
			rules = append(rules, rl)
		}
		re.mu.RUnlock()
		json.NewEncoder(w).Encode(map[string]interface{}{"rules": rules})

	case http.MethodPost:
		var rule Rule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			jsonError(w, "invalid rule", http.StatusBadRequest)
			return
		}
		re.AddRule(&rule)
		json.NewEncoder(w).Encode(map[string]string{"status": "created", "id": rule.ID})

	case http.MethodDelete:
		id := strings.TrimPrefix(r.URL.Path, "/api/rules/")
		re.RemoveRule(id)
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	}
}
