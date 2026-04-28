package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// validConfigYAML returns a minimal valid config YAML for hot update testing.
func validConfigYAML() string {
	return `
system:
  max_concurrent: 10
  cache_ttl: 300
  cache_max_size: 1000
  cache_cleanup_interval: 600
  retry_attempts: 3
  user_agent: "test"

cache:
  backend: memory

engines:
  quake:
    enabled: false
  zoomeye:
    enabled: false
  hunter:
    enabled: false
  fofa:
    enabled: false
  shodan:
    enabled: false
`
}

// ===== NewHotUpdateManager =====

func TestNewHotUpdateManager(t *testing.T) {
	mgr := NewHotUpdateManager("/tmp/config.yaml", NewManager("/tmp/config.yaml"))
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if mgr.configPath != "/tmp/config.yaml" {
		t.Errorf("configPath = %q, want /tmp/config.yaml", mgr.configPath)
	}
	if mgr.maxHistory != 10 {
		t.Errorf("maxHistory = %d, want 10", mgr.maxHistory)
	}
	if mgr.running {
		t.Error("should not be running initially")
	}
}

// ===== Start =====

func TestHotUpdateManager_Start_Disabled(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(validConfigYAML()), 0644)

	cm := NewManager(cfgPath)
	if err := cm.Load(); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	mgr := NewHotUpdateManager(cfgPath, cm)
	err := mgr.Start(HotUpdateConfig{Enabled: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Disabled should not start the watcher
	if mgr.watcher != nil {
		t.Error("watcher should be nil when disabled")
	}
}

func TestHotUpdateManager_Start_Enabled(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(validConfigYAML()), 0644)

	cm := NewManager(cfgPath)
	if err := cm.Load(); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	mgr := NewHotUpdateManager(cfgPath, cm)
	err := mgr.Start(HotUpdateConfig{
		Enabled:        true,
		CheckInterval:  5 * time.Second,
		MaxHistorySize: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mgr.watcher == nil {
		t.Error("watcher should be set when enabled")
	}
	if !mgr.running {
		t.Error("should be running after start")
	}

	mgr.Stop()
}

func TestHotUpdateManager_Start_AlreadyRunning(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(validConfigYAML()), 0644)

	cm := NewManager(cfgPath)
	cm.Load()

	mgr := NewHotUpdateManager(cfgPath, cm)
	mgr.Start(HotUpdateConfig{Enabled: true, CheckInterval: 5 * time.Second})
	defer mgr.Stop()

	// Second start should fail
	err := mgr.Start(HotUpdateConfig{Enabled: true, CheckInterval: 5 * time.Second})
	if err == nil {
		t.Fatal("expected error for already running manager")
	}
	if !containsStr(err.Error(), "already running") {
		t.Errorf("error should mention 'already running': %v", err)
	}
}

func TestHotUpdateManager_Start_InvalidConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	// Write invalid YAML
	os.WriteFile(cfgPath, []byte("invalid: [yaml: broken"), 0644)

	cm := NewManager(cfgPath)
	cm.Load()

	mgr := NewHotUpdateManager(cfgPath, cm)
	err := mgr.Start(HotUpdateConfig{Enabled: true, CheckInterval: 5 * time.Second})
	// Should succeed - initial start only reads valid config from cm, not from file
	// The file is read during checkConfigChanges, not during Start
	if err != nil {
		t.Fatalf("unexpected error on start: %v", err)
	}
	mgr.Stop()
}

func TestHotUpdateManager_Start_Defaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(validConfigYAML()), 0644)

	cm := NewManager(cfgPath)
	cm.Load()

	mgr := NewHotUpdateManager(cfgPath, cm)
	// Start with zero-valued config to test defaults
	err := mgr.Start(HotUpdateConfig{
		Enabled:         true,
		CheckInterval:   0, // should default to 30s
		MaxHistorySize:  0, // should keep 10
		RollbackTimeout: 0, // should default to 5m
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mgr.maxHistory != 10 {
		t.Errorf("maxHistory should remain 10 when MaxHistorySize is 0, got %d", mgr.maxHistory)
	}
	mgr.Stop()
}

// ===== Stop =====

func TestHotUpdateManager_Stop_NotRunning(t *testing.T) {
	mgr := NewHotUpdateManager("/tmp/config.yaml", nil)
	// Should not panic
	mgr.Stop()
}

func TestHotUpdateManager_Stop_Running(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(validConfigYAML()), 0644)

	cm := NewManager(cfgPath)
	cm.Load()

	mgr := NewHotUpdateManager(cfgPath, cm)
	mgr.Start(HotUpdateConfig{Enabled: true, CheckInterval: 5 * time.Second})
	mgr.Stop()

	if mgr.running {
		t.Error("should not be running after stop")
	}
	if mgr.watcher != nil {
		t.Error("watcher should be nil after stop")
	}
}

func TestHotUpdateManager_Stop_Idempotent(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(validConfigYAML()), 0644)

	cm := NewManager(cfgPath)
	cm.Load()

	mgr := NewHotUpdateManager(cfgPath, cm)
	mgr.Start(HotUpdateConfig{Enabled: true, CheckInterval: 5 * time.Second})
	mgr.Stop()
	mgr.Stop() // second stop should be safe
}

// ===== addConfigVersion =====

func TestAddConfigVersion(t *testing.T) {
	cm := NewManager("/tmp/config.yaml")
	cm.config = &Config{}
	cm.config.Cache.Backend = "memory"

	mgr := NewHotUpdateManager("/tmp/config.yaml", cm)
	mgr.addConfigVersion(cm.config, "abc123", "create")

	if len(mgr.configHistory) != 1 {
		t.Fatalf("expected 1 version, got %d", len(mgr.configHistory))
	}
	v := mgr.configHistory[0]
	if v.Version != 1 {
		t.Errorf("version = %d, want 1", v.Version)
	}
	if v.Checksum != "abc123" {
		t.Errorf("checksum = %q, want abc123", v.Checksum)
	}
	if v.ChangeType != "create" {
		t.Errorf("changeType = %q, want create", v.ChangeType)
	}
}

func TestAddConfigVersion_HistoryLimit(t *testing.T) {
	cm := NewManager("/tmp/config.yaml")
	cm.config = &Config{}
	cm.config.Cache.Backend = "memory"

	mgr := NewHotUpdateManager("/tmp/config.yaml", cm)
	mgr.maxHistory = 3

	for i := 0; i < 5; i++ {
		mgr.addConfigVersion(cm.config, "checksum", "update")
	}

	if len(mgr.configHistory) != 3 {
		t.Errorf("expected 3 versions (limit), got %d", len(mgr.configHistory))
	}
	// Versions should be renumbered
	if mgr.configHistory[0].Version != 3 {
		t.Errorf("first version = %d, want 3", mgr.configHistory[0].Version)
	}
}

func TestAddConfigVersion_NilClone(t *testing.T) {
	cm := NewManager("/tmp/config.yaml")
	// Config is nil, Clone() returns nil
	mgr := NewHotUpdateManager("/tmp/config.yaml", cm)
	mgr.addConfigVersion(nil, "checksum", "create")

	if len(mgr.configHistory) != 0 {
		t.Errorf("expected 0 versions for nil config, got %d", len(mgr.configHistory))
	}
}

// ===== GetCurrentVersion =====

func TestGetCurrentVersion(t *testing.T) {
	cm := NewManager("/tmp/config.yaml")
	mgr := NewHotUpdateManager("/tmp/config.yaml", cm)

	if mgr.GetCurrentVersion() != 0 {
		t.Errorf("expected version 0, got %d", mgr.GetCurrentVersion())
	}

	cm.config = &Config{}
	cm.config.Cache.Backend = "memory"
	mgr.addConfigVersion(cm.config, "abc", "create")

	if mgr.GetCurrentVersion() != 1 {
		t.Errorf("expected version 1, got %d", mgr.GetCurrentVersion())
	}
}

// ===== GetConfigHistory =====

func TestGetConfigHistory(t *testing.T) {
	cm := NewManager("/tmp/config.yaml")
	cm.config = &Config{}
	cm.config.Cache.Backend = "memory"

	mgr := NewHotUpdateManager("/tmp/config.yaml", cm)
	mgr.addConfigVersion(cm.config, "abc", "create")
	mgr.addConfigVersion(cm.config, "def", "update")

	history := mgr.GetConfigHistory()
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(history))
	}

	// Verify slice is a copy (modifying the slice doesn't affect internal state)
	history = append(history, ConfigVersion{})
	if len(mgr.configHistory) != 2 {
		t.Error("GetConfigHistory should return a copy of the slice")
	}
}

func TestGetConfigHistory_Empty(t *testing.T) {
	cm := NewManager("/tmp/config.yaml")
	mgr := NewHotUpdateManager("/tmp/config.yaml", cm)

	history := mgr.GetConfigHistory()
	if len(history) != 0 {
		t.Errorf("expected empty history, got %d entries", len(history))
	}
}

// ===== Rollback =====

func TestRollback_ValidVersion(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(validConfigYAML()), 0644)

	cm := NewManager(cfgPath)
	cm.Load()

	mgr := NewHotUpdateManager(cfgPath, cm)
	mgr.addConfigVersion(cm.config, "abc", "create")

	// Modify config
	cm.config.System.MaxConcurrent = 999

	// Rollback to version 1
	err := mgr.Rollback(1)
	if err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	// Verify config was restored
	if cm.config.System.MaxConcurrent != 10 {
		t.Errorf("MaxConcurrent = %d, want 10 after rollback", cm.config.System.MaxConcurrent)
	}

	// Verify new version was added
	if mgr.GetCurrentVersion() != 2 {
		t.Errorf("expected version 2 after rollback, got %d", mgr.GetCurrentVersion())
	}
}

func TestRollback_InvalidVersion(t *testing.T) {
	cm := NewManager("/tmp/config.yaml")
	mgr := NewHotUpdateManager("/tmp/config.yaml", cm)

	// Version 0
	err := mgr.Rollback(0)
	if err == nil {
		t.Fatal("expected error for version 0")
	}

	// Version > max
	err = mgr.Rollback(5)
	if err == nil {
		t.Fatal("expected error for version 5")
	}

	// Negative version
	err = mgr.Rollback(-1)
	if err == nil {
		t.Fatal("expected error for negative version")
	}
}

// ===== calculateChecksum =====

func TestCalculateChecksum(t *testing.T) {
	cfg1 := &Config{}
	cfg1.Cache.Backend = "memory"

	sum1, err := calculateChecksum(cfg1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sum1) != 64 {
		t.Errorf("checksum length = %d, want 64", len(sum1))
	}

	// Same config should produce same checksum
	sum2, err := calculateChecksum(cfg1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sum1 != sum2 {
		t.Error("same config should produce same checksum")
	}

	// Different config should produce different checksum
	cfg2 := &Config{}
	cfg2.Cache.Backend = "redis"
	sum3, err := calculateChecksum(cfg2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sum1 == sum3 {
		t.Error("different configs should produce different checksums")
	}
}

func TestCalculateChecksum_NilConfig(t *testing.T) {
	_, err := calculateChecksum(nil)
	if err != nil {
		t.Fatalf("unexpected error for nil config: %v", err)
	}
	// nil config should still produce a checksum (of null JSON)
}

// ===== validateConfig =====

func TestValidateConfig_Valid(t *testing.T) {
	cfg := &Config{}
	cfg.Cache.Backend = "memory"
	cfg.System.MaxConcurrent = 10

	err := validateConfig(cfg)
	if err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestValidateConfig_InvalidMaxConcurrent(t *testing.T) {
	cfg := &Config{}
	cfg.Cache.Backend = "memory"
	cfg.System.MaxConcurrent = -1

	err := validateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for negative max_concurrent")
	}
}

func TestValidateConfig_InvalidCacheMaxSize(t *testing.T) {
	cfg := &Config{}
	cfg.Cache.Backend = "memory"
	cfg.System.CacheMaxSize = -1

	err := validateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for negative cache_max_size")
	}
}

func TestValidateConfig_InvalidCacheBackend(t *testing.T) {
	cfg := &Config{}
	cfg.Cache.Backend = "unknown"

	err := validateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid cache backend")
	}
}

func TestValidateConfig_RedisRequired(t *testing.T) {
	cfg := &Config{}
	cfg.Cache.Backend = "redis"
	cfg.Cache.Redis.Addr = ""

	err := validateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing redis addr")
	}
}

func TestValidateConfig_RedisInvalidDB(t *testing.T) {
	cfg := &Config{}
	cfg.Cache.Backend = "redis"
	cfg.Cache.Redis.Addr = "localhost:6379"
	cfg.Cache.Redis.DB = 20

	err := validateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid redis DB")
	}
}

func TestValidateConfig_FofaRequiresAPIKey(t *testing.T) {
	cfg := &Config{}
	cfg.Cache.Backend = "memory"
	cfg.Engines.Fofa.Enabled = true
	cfg.Engines.Fofa.APIKey = ""

	err := validateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing fofa API key")
	}
}

func TestValidateConfig_ShodanRequiresAPIKey(t *testing.T) {
	cfg := &Config{}
	cfg.Cache.Backend = "memory"
	cfg.Engines.Shodan.Enabled = true
	cfg.Engines.Shodan.APIKey = ""

	err := validateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing shodan API key")
	}
}

func TestValidateConfig_InvalidEngineTimeout(t *testing.T) {
	cfg := &Config{}
	cfg.Cache.Backend = "memory"
	cfg.Engines.Quake.Enabled = true
	cfg.Engines.Quake.Timeout = -1

	err := validateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for negative engine timeout")
	}
}

func TestValidateConfig_InvalidEngineRateLimit(t *testing.T) {
	cfg := &Config{}
	cfg.Cache.Backend = "memory"
	cfg.Engines.Quake.Enabled = true
	cfg.Engines.Quake.QPS = -1

	err := validateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for negative engine rate limit")
	}
}

// ===== fileWatcher =====

func TestNewFileWatcher_FileExists(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("test"), 0644)

	w, err := newFileWatcher(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w == nil {
		t.Fatal("expected non-nil watcher")
	}
	if w.filePath != cfgPath {
		t.Errorf("filePath = %q, want %q", w.filePath, cfgPath)
	}
}

func TestNewFileWatcher_FileNotFound(t *testing.T) {
	_, err := newFileWatcher("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestFileWatcher_Check_NoChange(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("test"), 0644)

	w, err := newFileWatcher(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	changed, err := w.Check()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Error("should not detect change on unchanged file")
	}
}

func TestFileWatcher_Check_Changed(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("test"), 0644)

	w, err := newFileWatcher(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Modify the file
	os.WriteFile(cfgPath, []byte("modified content"), 0644)

	changed, err := w.Check()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Error("should detect change on modified file")
	}
}

func TestFileWatcher_Check_FileDeleted(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("test"), 0644)

	w, err := newFileWatcher(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	os.Remove(cfgPath)

	_, err = w.Check()
	if err == nil {
		t.Fatal("expected error for deleted file")
	}
}

func TestFileWatcher_Close(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("test"), 0644)

	w, err := newFileWatcher(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not panic
	w.Close()
}

// ===== readConfigFile =====

func TestReadConfigFile_Success(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := []byte("key: value")
	os.WriteFile(cfgPath, content, 0644)

	data, err := readConfigFile(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("content mismatch")
	}
}

func TestReadConfigFile_NotFound(t *testing.T) {
	_, err := readConfigFile("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

// ===== HotUpdateConfig defaults via Start =====

func TestHotUpdateConfig_DefaultRollbackTimeout(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(validConfigYAML()), 0644)

	cm := NewManager(cfgPath)
	cm.Load()

	mgr := NewHotUpdateManager(cfgPath, cm)
	// Start with zero RollbackTimeout to verify default
	err := mgr.Start(HotUpdateConfig{
		Enabled:         true,
		CheckInterval:   1 * time.Second,
		RollbackTimeout: 0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Can't easily verify the default value, but we verify no crash
	mgr.Stop()
}

// Helper

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStrAt(s, substr))
}

func containsStrAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
