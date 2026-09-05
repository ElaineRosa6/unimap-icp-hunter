package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuditAutoRollbackUpdateIdentity(t *testing.T) {
	for _, scenario := range []string{"current-control", "superseded", "single-history"} {
		t.Run(scenario, func(t *testing.T) {
			manager := NewManager("")
			old := &Config{}
			old.System.CacheTTL = 100
			manager.SetConfig(old)
			h := NewHotUpdateManager("", manager)
			h.running = true
			if scenario == "single-history" {
				h.maxHistory = 1
			}
			h.addConfigVersion(old, "old", "create")
			updated := old.Clone()
			updated.System.CacheTTL = 200
			manager.SetConfig(updated)
			h.addConfigVersion(updated, "updated", "update")
			updateVersion := h.GetCurrentVersion()
			want := 100
			if scenario == "superseded" {
				latest := old.Clone()
				latest.System.CacheTTL = 300
				manager.SetConfig(latest)
				h.addConfigVersion(latest, "latest", "update")
				want = 300
			}
			// Execute the timeout callback for the first update after the state
			// above. Zero delay avoids timing assumptions and background goroutines.
			h.scheduleRollback(0, old, updateVersion, h.stopChan)
			got := manager.GetConfig().System.CacheTTL
			if got != want {
				t.Fatalf("timeout for first update restored ttl=%d; want %d", got, want)
			}
		})
	}
}

func TestAutoRollbackLifecycleIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(validConfigYAML()), 0600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(path)
	if err := manager.Load(); err != nil {
		t.Fatal(err)
	}
	h := NewHotUpdateManager(path, manager)
	cfg := HotUpdateConfig{Enabled: true, CheckInterval: time.Hour}
	if err := h.Start(cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Stop)
	oldStop := h.stopChan
	old := manager.GetConfig()
	h.mutex.Lock()
	h.addConfigVersion(old, "fixture-update", "update")
	version := h.currentVersion
	h.mutex.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.scheduleRollback(time.Hour, old, version, oldStop)
	}()
	h.Stop()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stopped rollback still waiting on its hour-long timer")
	}
	if err := h.Start(cfg); err != nil {
		t.Fatal(err)
	}
	if h.stopChan == oldStop {
		t.Fatal("restart reused stopped lifecycle")
	}
	select {
	case <-h.stopChan:
		t.Fatal("restart stop channel already closed")
	default:
	}
	h.mutex.Lock()
	h.addConfigVersion(old, "force-current-read", "update")
	h.mutex.Unlock()
	before := h.GetCurrentVersion()
	h.checkConfigChanges(cfg, oldStop)
	h.scheduleRollback(0, old, before, oldStop)
	if h.GetCurrentVersion() != before {
		t.Fatal("stale monitor or callback published into restarted run")
	}
	// This same file read is accepted for the current lifecycle when its
	// checksum differs from the last record, proving the guard isn't blanket.
	h.mutex.Lock()
	h.configHistory[len(h.configHistory)-1].Checksum = "force-current-read"
	h.mutex.Unlock()
	h.checkConfigChanges(cfg, h.stopChan)
	if h.GetCurrentVersion() != before+1 {
		t.Fatal("current lifecycle stopped accepting updates")
	}
	h.Stop()
	h.Stop()
}
