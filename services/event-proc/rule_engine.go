package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
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
	logger *slog.Logger
}

func NewRuleEngine(logger *slog.Logger) *RuleEngine {
	return &RuleEngine{
		rules:  make(map[string]*Rule),
		logger: logger,
	}
}

func (re *RuleEngine) AddRule(rule *Rule) {
	re.mu.Lock()
	defer re.mu.Unlock()
	re.rules[rule.ID] = rule
}

func (re *RuleEngine) RemoveRule(id string) {
	re.mu.Lock()
	defer re.mu.Unlock()
	delete(re.rules, id)
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
