package config

import "testing"

func auditHistory(t *testing.T) *HotUpdateManager {
	t.Helper()
	manager := NewManager("")
	manager.SetConfig(&Config{})
	h := NewHotUpdateManager("", manager)
	h.maxHistory = 2
	for i := 1; i <= 4; i++ {
		cfg := &Config{}
		cfg.System.CacheTTL = i * 100
		h.addConfigVersion(cfg, "fixture", "update")
	}
	return h
}

func TestAuditHistoryVersionAfterEviction(t *testing.T) {
	h := auditHistory(t)
	entries := h.GetConfigHistory()
	if len(entries) != 2 {
		t.Fatal("retention length changed")
	}
	if entries[0].Version != 3 || entries[1].Version != 4 || h.GetCurrentVersion() != 4 {
		t.Fatalf("retained IDs=%d,%d current=%d; want 3,4 current=4", entries[0].Version, entries[1].Version, h.GetCurrentVersion())
	}
}

func TestAuditHistoryEvictedVersionRejected(t *testing.T) {
	h := auditHistory(t)
	if err := h.Rollback(1); err == nil {
		t.Fatalf("evicted ID 1 accepted and restored ttl=%d instead", h.configManager.GetConfig().System.CacheTTL)
	}
}

func TestAuditHistorySnapshotIsolation(t *testing.T) {
	h := auditHistory(t)
	entries := h.GetConfigHistory()
	entries[0].Config.System.CacheTTL = 9999
	if h.GetConfigHistory()[0].Config.System.CacheTTL == 9999 {
		t.Fatal("caller mutated stored rollback configuration through returned history")
	}
}

func TestHistoryRetainedRollbackKeepsIdentity(t *testing.T) {
	h := auditHistory(t)
	if err := h.Rollback(3); err != nil {
		t.Fatal(err)
	}
	if h.configManager.GetConfig().System.CacheTTL != 300 || h.GetCurrentVersion() != 5 {
		t.Fatal("rollback selected wrong config or failed to allocate a new identity")
	}
	history := h.GetConfigHistory()
	if history[0].Version != 4 || history[1].Version != 5 || history[1].ChangeType != "rollback" {
		t.Fatalf("unexpected retained rollback history: %+v", history)
	}
	if err := h.Rollback(3); err == nil {
		t.Fatal("rollback accepted an identity evicted by the preceding rollback")
	}
	if h.GetCurrentVersion() != 5 {
		t.Fatal("rejected rollback consumed a version")
	}
	if err := h.Rollback(4); err != nil {
		t.Fatal(err)
	}
	if h.GetCurrentVersion() != 6 || h.configManager.GetConfig().System.CacheTTL != 400 {
		t.Fatal("second retained rollback failed")
	}
}
