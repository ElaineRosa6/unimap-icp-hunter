package priority

import (
	"strings"
	"testing"
)

// --- Add/Remove Tests ---

func TestAddRule(t *testing.T) {
	pm := NewPriorityManager()

	t.Run("adds rule successfully", func(t *testing.T) {
		rule := &RulePriority{RuleID: "rule-1", Priority: 5, Enabled: true}
		if err := pm.AddRule(rule); err != nil {
			t.Fatalf("AddRule failed: %v", err)
		}
		got, ok := pm.GetRule("rule-1")
		if !ok {
			t.Fatal("expected rule to exist after add")
		}
		if got.Priority != 5 {
			t.Errorf("expected priority 5, got %d", got.Priority)
		}
	})

	t.Run("rejects empty rule ID", func(t *testing.T) {
		rule := &RulePriority{RuleID: "", Priority: 1}
		if err := pm.AddRule(rule); err == nil {
			t.Error("expected error for empty rule ID")
		}
	})
}

func TestRemoveRule(t *testing.T) {
	pm := NewPriorityManager()
	_ = pm.AddRule(&RulePriority{RuleID: "rule-1", Priority: 5, Enabled: true})

	t.Run("removes existing rule", func(t *testing.T) {
		if err := pm.RemoveRule("rule-1"); err != nil {
			t.Fatalf("RemoveRule failed: %v", err)
		}
		_, ok := pm.GetRule("rule-1")
		if ok {
			t.Error("expected rule to not exist after removal")
		}
	})

	t.Run("error removing nonexistent rule", func(t *testing.T) {
		err := pm.RemoveRule("no-such-rule")
		if err == nil {
			t.Error("expected error for nonexistent rule")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected 'not found' error, got %v", err)
		}
	})
}

func TestUpdateRule(t *testing.T) {
	pm := NewPriorityManager()
	_ = pm.AddRule(&RulePriority{RuleID: "rule-1", Priority: 5, Group: "group-a", Enabled: true})

	t.Run("updates priority", func(t *testing.T) {
		err := pm.UpdateRule(&RulePriority{RuleID: "rule-1", Priority: 10, Enabled: true})
		if err != nil {
			t.Fatalf("UpdateRule failed: %v", err)
		}
		rule, _ := pm.GetRule("rule-1")
		if rule.Priority != 10 {
			t.Errorf("expected priority 10, got %d", rule.Priority)
		}
	})

	t.Run("error for nonexistent rule", func(t *testing.T) {
		err := pm.UpdateRule(&RulePriority{RuleID: "missing", Priority: 1})
		if err == nil {
			t.Error("expected error for nonexistent rule")
		}
	})

	t.Run("empty rule ID rejected", func(t *testing.T) {
		err := pm.UpdateRule(&RulePriority{RuleID: "", Priority: 1})
		if err == nil {
			t.Error("expected error for empty rule ID")
		}
	})
}

// --- Group Tests ---

func TestRuleGroups(t *testing.T) {
	pm := NewPriorityManager()
	_ = pm.AddRule(&RulePriority{RuleID: "r1", Priority: 3, Group: "g1", Enabled: true})
	_ = pm.AddRule(&RulePriority{RuleID: "r2", Priority: 1, Group: "g1", Enabled: true})
	_ = pm.AddRule(&RulePriority{RuleID: "r3", Priority: 2, Group: "g2", Enabled: true})

	t.Run("get rules by group", func(t *testing.T) {
		rules := pm.GetRulesByGroup("g1")
		if len(rules) != 2 {
			t.Fatalf("expected 2 rules in g1, got %d", len(rules))
		}
		// Should be sorted by priority (ascending = highest priority first)
		if rules[0].RuleID != "r2" || rules[1].RuleID != "r1" {
			t.Errorf("expected sorted order [r2, r1], got %v", []string{rules[0].RuleID, rules[1].RuleID})
		}
	})

	t.Run("empty slice for unknown group", func(t *testing.T) {
		rules := pm.GetRulesByGroup("nonexistent")
		if rules == nil {
			t.Error("expected empty slice, got nil")
		}
		if len(rules) != 0 {
			t.Errorf("expected 0 rules, got %d", len(rules))
		}
	})

	t.Run("get all groups", func(t *testing.T) {
		groups := pm.GetGroups()
		if len(groups) != 2 {
			t.Errorf("expected 2 groups, got %d: %v", len(groups), groups)
		}
	})
}

func TestUpdateRuleMovesGroup(t *testing.T) {
	pm := NewPriorityManager()
	_ = pm.AddRule(&RulePriority{RuleID: "r1", Priority: 1, Group: "old-group", Enabled: true})

	_ = pm.UpdateRule(&RulePriority{RuleID: "r1", Priority: 1, Group: "new-group", Enabled: true})

	oldRules := pm.GetRulesByGroup("old-group")
	if len(oldRules) != 0 {
		t.Errorf("expected rule removed from old-group, got %d rules", len(oldRules))
	}

	newRules := pm.GetRulesByGroup("new-group")
	if len(newRules) != 1 {
		t.Fatalf("expected 1 rule in new-group, got %d", len(newRules))
	}
}

// --- Priority Ordering Tests ---

func TestGetRulesByPriority(t *testing.T) {
	pm := NewPriorityManager()
	_ = pm.AddRule(&RulePriority{RuleID: "low", Priority: 10, Enabled: true})
	_ = pm.AddRule(&RulePriority{RuleID: "high", Priority: 1, Enabled: true})
	_ = pm.AddRule(&RulePriority{RuleID: "medium", Priority: 5, Enabled: true})
	_ = pm.AddRule(&RulePriority{RuleID: "disabled", Priority: 2, Enabled: false})

	rules := pm.GetRulesByPriority()
	if len(rules) != 3 {
		t.Fatalf("expected 3 enabled rules, got %d", len(rules))
	}
	// Lower number = higher priority
	if rules[0].RuleID != "high" || rules[1].RuleID != "medium" || rules[2].RuleID != "low" {
		t.Errorf("wrong order: %v", rules)
	}
}

// --- Enable/Disable Tests ---

func TestEnableDisableRule(t *testing.T) {
	pm := NewPriorityManager()
	_ = pm.AddRule(&RulePriority{RuleID: "r1", Priority: 1, Enabled: true})

	t.Run("disable", func(t *testing.T) {
		if err := pm.DisableRule("r1"); err != nil {
			t.Fatalf("DisableRule failed: %v", err)
		}
		if pm.IsRuleEnabled("r1") {
			t.Error("expected rule to be disabled")
		}
	})

	t.Run("enable", func(t *testing.T) {
		if err := pm.EnableRule("r1"); err != nil {
			t.Fatalf("EnableRule failed: %v", err)
		}
		if !pm.IsRuleEnabled("r1") {
			t.Error("expected rule to be enabled")
		}
	})

	t.Run("nonexistent rule", func(t *testing.T) {
		if err := pm.EnableRule("missing"); err == nil {
			t.Error("expected error for nonexistent rule")
		}
		if err := pm.DisableRule("missing"); err == nil {
			t.Error("expected error for nonexistent rule")
		}
	})
}

func TestSetRulePriority(t *testing.T) {
	pm := NewPriorityManager()
	_ = pm.AddRule(&RulePriority{RuleID: "r1", Priority: 5})

	t.Run("set priority", func(t *testing.T) {
		if err := pm.SetRulePriority("r1", 10); err != nil {
			t.Fatalf("SetRulePriority failed: %v", err)
		}
		rule, _ := pm.GetRule("r1")
		if rule.Priority != 10 {
			t.Errorf("expected priority 10, got %d", rule.Priority)
		}
	})

	t.Run("nonexistent rule", func(t *testing.T) {
		if err := pm.SetRulePriority("missing", 5); err == nil {
			t.Error("expected error for nonexistent rule")
		}
	})
}

func TestGetAllRules(t *testing.T) {
	pm := NewPriorityManager()
	_ = pm.AddRule(&RulePriority{RuleID: "r1", Enabled: true})
	_ = pm.AddRule(&RulePriority{RuleID: "r2", Enabled: false})

	all := pm.GetAllRules()
	if len(all) != 2 {
		t.Errorf("expected 2 total rules, got %d", len(all))
	}
}

// --- Priority Range Tests ---

func TestGetRulesByPriorityRange(t *testing.T) {
	pm := NewPriorityManager()
	_ = pm.AddRule(&RulePriority{RuleID: "r1", Priority: 1, Enabled: true})
	_ = pm.AddRule(&RulePriority{RuleID: "r2", Priority: 5, Enabled: true})
	_ = pm.AddRule(&RulePriority{RuleID: "r3", Priority: 10, Enabled: true})
	_ = pm.AddRule(&RulePriority{RuleID: "r4", Priority: 3, Enabled: false})

	rules := pm.GetRulesByPriorityRange(2, 6)
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule in range [2,6], got %d", len(rules))
	}
	if rules[0].RuleID != "r2" {
		t.Errorf("expected r2, got %s", rules[0].RuleID)
	}
}

// --- Conflict Tests ---

func TestConflictDetection(t *testing.T) {
	pm := NewPriorityManager()
	// Add the excluded rule first, so when r1 is added, the conflict is detected
	_ = pm.AddRule(&RulePriority{RuleID: "r2", Priority: 3, Enabled: true})
	_ = pm.AddRule(&RulePriority{RuleID: "r1", Priority: 5, Exclusions: []string{"r2"}, Enabled: true})

	conflicts := pm.GetConflicts()
	if _, exists := conflicts["r1"]; !exists {
		t.Error("expected conflict for r1 (excludes r2)")
	}
}

func TestMissingDependencyConflict(t *testing.T) {
	pm := NewPriorityManager()
	_ = pm.AddRule(&RulePriority{RuleID: "r1", Priority: 5, Dependencies: []string{"missing-dep"}, Enabled: true})

	conflicts := pm.GetConflicts()
	if len(conflicts) == 0 {
		t.Error("expected missing dependency conflict")
	}
}

func TestResolveConflict(t *testing.T) {
	pm := NewPriorityManager()
	_ = pm.AddRule(&RulePriority{RuleID: "r2", Priority: 3, Enabled: true})
	_ = pm.AddRule(&RulePriority{RuleID: "r1", Priority: 5, Exclusions: []string{"r2"}, Enabled: true})

	t.Run("resolve existing conflict", func(t *testing.T) {
		if err := pm.ResolveConflict("r1", "r1 wins"); err != nil {
			t.Fatalf("ResolveConflict failed: %v", err)
		}
	})

	t.Run("error resolving nonexistent conflict", func(t *testing.T) {
		if err := pm.ResolveConflict("nonexistent", "resolved"); err == nil {
			t.Error("expected error for nonexistent conflict")
		}
	})
}

func TestRemoveConflict(t *testing.T) {
	pm := NewPriorityManager()
	_ = pm.AddRule(&RulePriority{RuleID: "r2", Priority: 3, Enabled: true})
	_ = pm.AddRule(&RulePriority{RuleID: "r1", Priority: 5, Exclusions: []string{"r2"}, Enabled: true})

	pm.RemoveConflict("r1")
	conflicts := pm.GetConflicts()
	if _, exists := conflicts["r1"]; exists {
		t.Error("expected conflict for r1 to be removed")
	}
}

func TestRemoveConflictsByRuleID(t *testing.T) {
	pm := NewPriorityManager()
	_ = pm.AddRule(&RulePriority{RuleID: "r2", Priority: 3, Enabled: true})
	_ = pm.AddRule(&RulePriority{RuleID: "r1", Priority: 5, Exclusions: []string{"r2"}, Enabled: true})
	_ = pm.AddRule(&RulePriority{RuleID: "r3", Priority: 1, Exclusions: []string{"r2"}, Enabled: true})

	pm.RemoveConflictsByRuleID("r2")
	conflicts := pm.GetConflicts()
	for _, c := range conflicts {
		for _, w := range c.ConflictsWith {
			if w == "r2" {
				t.Error("expected all conflicts with r2 to be removed")
			}
		}
	}
}

// --- Concurrent Access Tests ---

func TestPriorityManagerConcurrency(t *testing.T) {
	pm := NewPriorityManager()
	done := make(chan bool)

	for i := 0; i < 20; i++ {
		go func(id int) {
			ruleID := "r-" + string(rune('0'+id))
			_ = pm.AddRule(&RulePriority{RuleID: ruleID, Priority: id, Enabled: true})
			pm.GetRule(ruleID)
			pm.GetAllRules()
			pm.GetRulesByPriority()
			pm.GetConflicts()
			pm.IsRuleEnabled(ruleID)
			_ = pm.EnableRule(ruleID)
			_ = pm.SetRulePriority(ruleID, id+10)
			done <- true
		}(i)
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}
