package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAuditHotLoadParity(t *testing.T) {
	t.Setenv("UNIMAP_AUDIT_USER_AGENT", "fixture-agent")
	for _, scenario := range []string{"environment", "defaults", "invalid-mode"} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			manager := NewManager(path)
			initial := &Config{}
			manager.ApplyDefaults(initial)
			manager.SetConfig(initial)
			candidate := initial.Clone()
			candidate.System.CacheTTL = 7200
			switch scenario {
			case "environment":
				candidate.System.UserAgent = "${UNIMAP_AUDIT_USER_AGENT}"
			case "defaults":
				candidate.Screenshot.Mode = ""
			case "invalid-mode":
				candidate.Screenshot.Mode = "invalid-fixture-mode"
			}
			data, err := yaml.Marshal(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			startup := NewManager(path)
			loadErr := startup.Load()
			h := NewHotUpdateManager(path, manager)
			h.running = true
			h.addConfigVersion(initial, "initial", "create")
			h.checkConfigChanges(HotUpdateConfig{}, h.stopChan)
			got := manager.GetConfig()
			if scenario == "invalid-mode" {
				if loadErr == nil {
					t.Fatal("invalid startup control unexpectedly accepted")
				}
				if got.Screenshot.Mode != initial.Screenshot.Mode || h.GetCurrentVersion() != 1 {
					t.Fatal("hot update published configuration rejected by startup validation")
				}
				return
			}
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			want := startup.GetConfig()
			if got.System.UserAgent != want.System.UserAgent || got.Screenshot.Mode != want.Screenshot.Mode {
				t.Fatalf("load parity differs: hot agent=%q mode=%q; startup agent=%q mode=%q", got.System.UserAgent, got.Screenshot.Mode, want.System.UserAgent, want.Screenshot.Mode)
			}
			if got.System.CacheTTL != 7200 {
				t.Fatal("valid hot update was ignored")
			}
			version := h.GetCurrentVersion()
			h.checkConfigChanges(HotUpdateConfig{}, h.stopChan)
			if h.GetCurrentVersion() != version {
				t.Fatal("unchanged normalized configuration created another version")
			}
			if err = os.WriteFile(path, []byte("broken: ["), 0600); err != nil {
				t.Fatal(err)
			}
			h.checkConfigChanges(HotUpdateConfig{}, h.stopChan)
			if h.GetCurrentVersion() != version || manager.GetConfig().System.CacheTTL != 7200 {
				t.Fatal("malformed update replaced committed configuration")
			}
		})
	}
}
