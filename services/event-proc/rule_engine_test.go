package main

import (
	"testing"
)

func TestSimpleRuleMatch(t *testing.T) {
	re := NewRuleEngine(nil)
	re.AddRule(&Rule{
		ID: "test1", Enabled: true,
		Conditions: []Condition{
			{Source: "object_type", Operator: "equals", Value: "person"},
		},
		Actions: []Action{{Type: "alert", Params: map[string]string{"message": "test"}}},
		Logic:   "AND",
	})

	actions := re.Evaluate(map[string]interface{}{
		"camera_id":   "cam1",
		"object_type": "person",
		"confidence":  0.95,
	})
	if len(actions) == 0 {
		t.Fatal("expected rule to match")
	}
}

func TestRuleNoMatch(t *testing.T) {
	re := NewRuleEngine(nil)
	re.AddRule(&Rule{
		ID: "test2", Enabled: true,
		Conditions: []Condition{
			{Source: "object_type", Operator: "equals", Value: "car"},
		},
	})

	actions := re.Evaluate(map[string]interface{}{
		"camera_id":   "cam1",
		"object_type": "person",
	})
	if len(actions) > 0 {
		t.Fatal("expected no match")
	}
}

func TestORLogic(t *testing.T) {
	re := NewRuleEngine(nil)
	re.AddRule(&Rule{
		ID: "test3", Enabled: true, Logic: "OR",
		Conditions: []Condition{
			{Source: "object_type", Operator: "equals", Value: "person"},
			{Source: "object_type", Operator: "equals", Value: "car"},
		},
		Actions: []Action{{Type: "alert", Params: map[string]string{"message": "test"}}},
	})

	actions := re.Evaluate(map[string]interface{}{"object_type": "car"})
	if len(actions) == 0 {
		t.Fatal("expected OR rule to match car")
	}
}

func TestDisabledRule(t *testing.T) {
	re := NewRuleEngine(nil)
	re.AddRule(&Rule{
		ID: "test4", Enabled: false,
		Conditions: []Condition{
			{Source: "object_type", Operator: "equals", Value: "person"},
		},
	})

	actions := re.Evaluate(map[string]interface{}{"object_type": "person"})
	if len(actions) > 0 {
		t.Fatal("expected disabled rule not to match")
	}
}
