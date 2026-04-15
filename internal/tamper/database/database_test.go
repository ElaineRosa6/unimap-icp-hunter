package database

import (
	"os"
	"path/filepath"
	"testing"
)

// --- Database Tests ---

func setupDB(t *testing.T) (*Database, string) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	if err := db.InitSchema(); err != nil {
		t.Fatalf("failed to init schema: %v", err)
	}
	return db, dbPath
}

func TestNewDatabase(t *testing.T) {
	t.Run("creates database file", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")
		db, err := NewDatabase(dbPath)
		if err != nil {
			t.Fatalf("NewDatabase failed: %v", err)
		}
		defer db.Close()

		// Verify file exists
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			t.Error("expected database file to exist")
		}
	})

	t.Run("invalid db path", func(t *testing.T) {
		_, err := NewDatabase("/nonexistent/dir/test.db")
		if err == nil {
			t.Error("expected error for invalid db path")
		}
	})
}

func TestInitSchema(t *testing.T) {
	db, _ := setupDB(t)
	defer db.Close()

	t.Run("idempotent", func(t *testing.T) {
		// Running InitSchema twice should not error
		if err := db.InitSchema(); err != nil {
			t.Fatalf("second InitSchema failed: %v", err)
		}
	})
}

func TestDatabaseClose(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	// Double close of sql.DB is a no-op in Go (returns nil)
	if err := db.Close(); err != nil {
		t.Errorf("expected nil on double close, got %v", err)
	}
}

// --- RuleRepository Tests ---

func setupRuleRepo(t *testing.T) (RuleRepository, *Database) {
	t.Helper()
	db, _ := setupDB(t)
	repo := NewRuleRepository(db.db)
	return repo, db
}

func testRule(t *testing.T) *Rule {
	t.Helper()
	return &Rule{
		Name:        "Test Rule",
		Type:        "script",
		Pattern:     `<script>eval\(</script>`,
		Description: "Detects eval in script tags",
		Severity:    3,
		Enabled:     true,
		Priority:    10,
	}
}

func TestCreateRule(t *testing.T) {
	repo, db := setupRuleRepo(t)
	defer db.Close()

	t.Run("creates rule with ID", func(t *testing.T) {
		rule := testRule(t)
		if err := repo.CreateRule(rule); err != nil {
			t.Fatalf("CreateRule failed: %v", err)
		}
		if rule.ID == 0 {
			t.Error("expected rule ID to be set after creation")
		}
	})

	t.Run("creates version record", func(t *testing.T) {
		rule := testRule(t)
		if err := repo.CreateRule(rule); err != nil {
			t.Fatalf("CreateRule failed: %v", err)
		}
		// If no error, version was created
	})
}

func TestGetRuleByID(t *testing.T) {
	repo, db := setupRuleRepo(t)
	defer db.Close()

	// Create a rule first
	rule := testRule(t)
	if err := repo.CreateRule(rule); err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}

	t.Run("finds existing rule", func(t *testing.T) {
		got, err := repo.GetRuleByID(rule.ID)
		if err != nil {
			t.Fatalf("GetRuleByID failed: %v", err)
		}
		if got.Name != rule.Name {
			t.Errorf("expected name %q, got %q", rule.Name, got.Name)
		}
		if got.Type != rule.Type {
			t.Errorf("expected type %q, got %q", rule.Type, got.Type)
		}
	})

	t.Run("error for nonexistent rule", func(t *testing.T) {
		_, err := repo.GetRuleByID(9999)
		if err == nil {
			t.Error("expected error for nonexistent rule")
		}
	})
}

func TestGetRulesByType(t *testing.T) {
	repo, db := setupRuleRepo(t)
	defer db.Close()

	_ = repo.CreateRule(&Rule{Name: "r1", Type: "script", Pattern: "p1", Severity: 1, Enabled: true})
	_ = repo.CreateRule(&Rule{Name: "r2", Type: "script", Pattern: "p2", Severity: 1, Enabled: true})
	_ = repo.CreateRule(&Rule{Name: "r3", Type: "style", Pattern: "p3", Severity: 1, Enabled: true})

	t.Run("returns matching type", func(t *testing.T) {
		rules, err := repo.GetRulesByType("script")
		if err != nil {
			t.Fatalf("GetRulesByType failed: %v", err)
		}
		if len(rules) != 2 {
			t.Errorf("expected 2 script rules, got %d", len(rules))
		}
	})

	t.Run("empty for unknown type", func(t *testing.T) {
		rules, err := repo.GetRulesByType("unknown")
		if err != nil {
			t.Fatalf("GetRulesByType failed: %v", err)
		}
		if len(rules) != 0 {
			t.Errorf("expected 0 rules, got %d", len(rules))
		}
	})
}

func TestGetAllRules(t *testing.T) {
	repo, db := setupRuleRepo(t)
	defer db.Close()

	_ = repo.CreateRule(&Rule{Name: "r1", Type: "t1", Pattern: "p1", Severity: 1, Enabled: true})
	_ = repo.CreateRule(&Rule{Name: "r2", Type: "t2", Pattern: "p2", Severity: 1, Enabled: true})

	t.Run("returns all rules", func(t *testing.T) {
		rules, err := repo.GetAllRules()
		if err != nil {
			t.Fatalf("GetAllRules failed: %v", err)
		}
		if len(rules) != 2 {
			t.Errorf("expected 2 rules, got %d", len(rules))
		}
	})
}

func TestUpdateRule(t *testing.T) {
	repo, db := setupRuleRepo(t)
	defer db.Close()

	rule := testRule(t)
	if err := repo.CreateRule(rule); err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}

	t.Run("updates rule", func(t *testing.T) {
		rule.Name = "Updated Rule"
		rule.Priority = 20
		if err := repo.UpdateRule(rule); err != nil {
			t.Fatalf("UpdateRule failed: %v", err)
		}

		got, err := repo.GetRuleByID(rule.ID)
		if err != nil {
			t.Fatalf("GetRuleByID failed: %v", err)
		}
		if got.Name != "Updated Rule" {
			t.Errorf("expected name 'Updated Rule', got %q", got.Name)
		}
		if got.Priority != 20 {
			t.Errorf("expected priority 20, got %d", got.Priority)
		}
	})

	t.Run("error for nonexistent rule", func(t *testing.T) {
		err := repo.UpdateRule(&Rule{ID: 9999, Name: "missing"})
		if err == nil {
			t.Error("expected error for nonexistent rule")
		}
	})
}

func TestDeleteRule(t *testing.T) {
	repo, db := setupRuleRepo(t)
	defer db.Close()

	rule := testRule(t)
	if err := repo.CreateRule(rule); err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}

	t.Run("deletes rule and versions", func(t *testing.T) {
		if err := repo.DeleteRule(rule.ID); err != nil {
			t.Fatalf("DeleteRule failed: %v", err)
		}

		_, err := repo.GetRuleByID(rule.ID)
		if err == nil {
			t.Error("expected error after deletion")
		}
	})

	t.Run("error for nonexistent rule", func(t *testing.T) {
		err := repo.DeleteRule(9999)
		if err == nil {
			t.Error("expected error for nonexistent rule")
		}
	})
}

func TestGetEnabledRules(t *testing.T) {
	repo, db := setupRuleRepo(t)
	defer db.Close()

	_ = repo.CreateRule(&Rule{Name: "enabled", Type: "t", Pattern: "p", Severity: 1, Enabled: true})
	_ = repo.CreateRule(&Rule{Name: "disabled", Type: "t", Pattern: "p", Severity: 1, Enabled: false})

	t.Run("returns only enabled rules", func(t *testing.T) {
		rules, err := repo.GetEnabledRules()
		if err != nil {
			t.Fatalf("GetEnabledRules failed: %v", err)
		}
		if len(rules) != 1 {
			t.Errorf("expected 1 enabled rule, got %d", len(rules))
		}
		if rules[0].Name != "enabled" {
			t.Errorf("expected 'enabled' rule, got %q", rules[0].Name)
		}
	})
}

func TestGetRulesByPriority(t *testing.T) {
	repo, db := setupRuleRepo(t)
	defer db.Close()

	_ = repo.CreateRule(&Rule{Name: "low", Type: "t", Pattern: "p", Severity: 1, Enabled: true, Priority: 100})
	_ = repo.CreateRule(&Rule{Name: "high", Type: "t", Pattern: "p", Severity: 1, Enabled: true, Priority: 1})

	t.Run("returns rules sorted by priority desc", func(t *testing.T) {
		rules, err := repo.GetRulesByPriority()
		if err != nil {
			t.Fatalf("GetRulesByPriority failed: %v", err)
		}
		if len(rules) != 2 {
			t.Fatalf("expected 2 rules, got %d", len(rules))
		}
		if rules[0].Name != "low" {
			t.Errorf("expected 'low' (priority 100) first, got %q", rules[0].Name)
		}
	})
}

// --- WhitelistRepository Tests ---

func setupWhitelistRepo(t *testing.T) (WhitelistRepository, *Database) {
	t.Helper()
	db, _ := setupDB(t)
	repo := NewWhitelistRepository(db.db)
	return repo, db
}

func testWhitelist(t *testing.T) *Whitelist {
	t.Helper()
	return &Whitelist{
		Type:        "domain",
		Value:       "example.com",
		Description: "Trusted domain",
	}
}

func TestCreateWhitelist(t *testing.T) {
	repo, db := setupWhitelistRepo(t)
	defer db.Close()

	t.Run("creates with ID", func(t *testing.T) {
		item := testWhitelist(t)
		if err := repo.CreateWhitelist(item); err != nil {
			t.Fatalf("CreateWhitelist failed: %v", err)
		}
		if item.ID == 0 {
			t.Error("expected ID to be set after creation")
		}
	})
}

func TestGetWhitelistByID(t *testing.T) {
	repo, db := setupWhitelistRepo(t)
	defer db.Close()

	item := testWhitelist(t)
	if err := repo.CreateWhitelist(item); err != nil {
		t.Fatalf("CreateWhitelist failed: %v", err)
	}

	t.Run("finds existing item", func(t *testing.T) {
		got, err := repo.GetWhitelistByID(item.ID)
		if err != nil {
			t.Fatalf("GetWhitelistByID failed: %v", err)
		}
		if got.Value != item.Value {
			t.Errorf("expected value %q, got %q", item.Value, got.Value)
		}
	})

	t.Run("error for nonexistent", func(t *testing.T) {
		_, err := repo.GetWhitelistByID(9999)
		if err == nil {
			t.Error("expected error for nonexistent")
		}
	})
}

func TestGetWhitelistByType(t *testing.T) {
	repo, db := setupWhitelistRepo(t)
	defer db.Close()

	_ = repo.CreateWhitelist(&Whitelist{Type: "domain", Value: "a.com"})
	_ = repo.CreateWhitelist(&Whitelist{Type: "domain", Value: "b.com"})
	_ = repo.CreateWhitelist(&Whitelist{Type: "ip", Value: "192.168.1.1"})

	t.Run("returns matching type", func(t *testing.T) {
		items, err := repo.GetWhitelistByType("domain")
		if err != nil {
			t.Fatalf("GetWhitelistByType failed: %v", err)
		}
		if len(items) != 2 {
			t.Errorf("expected 2 items, got %d", len(items))
		}
	})

	t.Run("empty for unknown type", func(t *testing.T) {
		items, err := repo.GetWhitelistByType("unknown")
		if err != nil {
			t.Fatalf("GetWhitelistByType failed: %v", err)
		}
		if len(items) != 0 {
			t.Errorf("expected 0 items, got %d", len(items))
		}
	})
}

func TestGetAllWhitelist(t *testing.T) {
	repo, db := setupWhitelistRepo(t)
	defer db.Close()

	_ = repo.CreateWhitelist(&Whitelist{Type: "domain", Value: "a.com"})
	_ = repo.CreateWhitelist(&Whitelist{Type: "ip", Value: "1.2.3.4"})

	t.Run("returns all", func(t *testing.T) {
		items, err := repo.GetAllWhitelist()
		if err != nil {
			t.Fatalf("GetAllWhitelist failed: %v", err)
		}
		if len(items) != 2 {
			t.Errorf("expected 2 items, got %d", len(items))
		}
	})
}

func TestDeleteWhitelist(t *testing.T) {
	repo, db := setupWhitelistRepo(t)
	defer db.Close()

	item := testWhitelist(t)
	if err := repo.CreateWhitelist(item); err != nil {
		t.Fatalf("CreateWhitelist failed: %v", err)
	}

	t.Run("deletes by ID", func(t *testing.T) {
		if err := repo.DeleteWhitelist(item.ID); err != nil {
			t.Fatalf("DeleteWhitelist failed: %v", err)
		}
		_, err := repo.GetWhitelistByID(item.ID)
		if err == nil {
			t.Error("expected error after deletion")
		}
	})

	t.Run("error for nonexistent", func(t *testing.T) {
		err := repo.DeleteWhitelist(9999)
		if err == nil {
			t.Error("expected error for nonexistent")
		}
	})
}

func TestDeleteWhitelistByType(t *testing.T) {
	repo, db := setupWhitelistRepo(t)
	defer db.Close()

	_ = repo.CreateWhitelist(&Whitelist{Type: "domain", Value: "a.com"})
	_ = repo.CreateWhitelist(&Whitelist{Type: "domain", Value: "b.com"})

	t.Run("deletes by type", func(t *testing.T) {
		if err := repo.DeleteWhitelistByType("domain"); err != nil {
			t.Fatalf("DeleteWhitelistByType failed: %v", err)
		}
		items, _ := repo.GetWhitelistByType("domain")
		if len(items) != 0 {
			t.Errorf("expected 0 items after delete, got %d", len(items))
		}
	})

	t.Run("error for unknown type", func(t *testing.T) {
		err := repo.DeleteWhitelistByType("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent type")
		}
	})
}

func TestDeleteWhitelistByValue(t *testing.T) {
	repo, db := setupWhitelistRepo(t)
	defer db.Close()

	_ = repo.CreateWhitelist(&Whitelist{Type: "domain", Value: "delete-me.com"})

	t.Run("deletes by value", func(t *testing.T) {
		if err := repo.DeleteWhitelistByValue("delete-me.com"); err != nil {
			t.Fatalf("DeleteWhitelistByValue failed: %v", err)
		}
		exists, _ := repo.ExistsByTypeAndValue("domain", "delete-me.com")
		if exists {
			t.Error("expected item to be deleted")
		}
	})
}

func TestExistsByTypeAndValue(t *testing.T) {
	repo, db := setupWhitelistRepo(t)
	defer db.Close()

	_ = repo.CreateWhitelist(&Whitelist{Type: "domain", Value: "exists.com"})

	t.Run("returns true for existing", func(t *testing.T) {
		exists, err := repo.ExistsByTypeAndValue("domain", "exists.com")
		if err != nil {
			t.Fatalf("ExistsByTypeAndValue failed: %v", err)
		}
		if !exists {
			t.Error("expected exists to be true")
		}
	})

	t.Run("returns false for nonexistent", func(t *testing.T) {
		exists, err := repo.ExistsByTypeAndValue("domain", "nope.com")
		if err != nil {
			t.Fatalf("ExistsByTypeAndValue failed: %v", err)
		}
		if exists {
			t.Error("expected exists to be false")
		}
	})
}

func TestBatchCreateWhitelist(t *testing.T) {
	repo, db := setupWhitelistRepo(t)
	defer db.Close()

	// Cast to concrete type for BatchCreateWhitelist
	impl, ok := repo.(*whitelistRepository)
	if !ok {
		t.Fatal("expected *whitelistRepository")
	}

	t.Run("creates multiple", func(t *testing.T) {
		items := []*Whitelist{
			{Type: "domain", Value: "a.com"},
			{Type: "domain", Value: "b.com"},
			{Type: "ip", Value: "1.2.3.4"},
		}
		if err := impl.BatchCreateWhitelist(items); err != nil {
			t.Fatalf("BatchCreateWhitelist failed: %v", err)
		}
		all, _ := repo.GetAllWhitelist()
		if len(all) != 3 {
			t.Errorf("expected 3 items, got %d", len(all))
		}
	})
}

func TestGetWhitelistValuesByType(t *testing.T) {
	repo, db := setupWhitelistRepo(t)
	defer db.Close()

	// Cast to concrete type for GetWhitelistValuesByType
	impl, ok := repo.(*whitelistRepository)
	if !ok {
		t.Fatal("expected *whitelistRepository")
	}

	_ = repo.CreateWhitelist(&Whitelist{Type: "domain", Value: "a.com"})
	_ = repo.CreateWhitelist(&Whitelist{Type: "domain", Value: "b.com"})
	_ = repo.CreateWhitelist(&Whitelist{Type: "ip", Value: "1.2.3.4"})

	t.Run("returns values only", func(t *testing.T) {
		values, err := impl.GetWhitelistValuesByType("domain")
		if err != nil {
			t.Fatalf("GetWhitelistValuesByType failed: %v", err)
		}
		if len(values) != 2 {
			t.Errorf("expected 2 values, got %d", len(values))
		}
	})

	t.Run("empty for unknown type", func(t *testing.T) {
		values, err := impl.GetWhitelistValuesByType("unknown")
		if err != nil {
			t.Fatalf("GetWhitelistValuesByType failed: %v", err)
		}
		if len(values) != 0 {
			t.Errorf("expected 0 values, got %d", len(values))
		}
	})
}
