package utils

import (
	"strings"
	"testing"
	"time"
)

// ===== ConfigBasedCacheStrategy: IsCacheEnabledForEngine =====

func TestConfigBasedCacheStrategy_IsCacheEnabledForEngine(t *testing.T) {
	s := NewConfigBasedCacheStrategy(30 * time.Minute)

	// Default: enabled for unknown engines
	if !s.IsCacheEnabledForEngine("unknown") {
		t.Error("expected cache enabled by default for unknown engine")
	}

	// Configure an engine as disabled
	s.SetEngineConfig("disabled_engine", &SimpleEngineCacheConfig{
		Enabled: false,
		TTL:     5 * time.Minute,
		MaxSize: 100,
	})
	if s.IsCacheEnabledForEngine("disabled_engine") {
		t.Error("expected cache disabled for disabled_engine")
	}

	// Case insensitive: DISABLED_ENGINE should resolve to disabled_engine config
	if s.IsCacheEnabledForEngine("DISABLED_ENGINE") {
		t.Error("expected case-insensitive lookup to return disabled status")
	}

	// Configure an engine as enabled
	s.SetEngineConfig("enabled_engine", &SimpleEngineCacheConfig{
		Enabled: true,
		TTL:     10 * time.Minute,
		MaxSize: 500,
	})
	// Case insensitive for enabled engine
	if !s.IsCacheEnabledForEngine("ENABLED_ENGINE") {
		t.Error("expected case-insensitive lookup for enabled engine")
	}
}

// ===== ConfigBasedCacheStrategy: GetMaxSizeForEngine =====

func TestConfigBasedCacheStrategy_GetMaxSizeForEngine(t *testing.T) {
	s := NewConfigBasedCacheStrategy(30 * time.Minute)

	// Default value for unknown engine
	if size := s.GetMaxSizeForEngine("unknown"); size != 1000 {
		t.Errorf("expected default size 1000, got %d", size)
	}

	// Configured with custom max size
	s.SetEngineConfig("small_engine", &SimpleEngineCacheConfig{
		Enabled: true,
		TTL:     5 * time.Minute,
		MaxSize: 50,
	})
	if size := s.GetMaxSizeForEngine("small_engine"); size != 50 {
		t.Errorf("expected size 50, got %d", size)
	}

	// Config with zero max size should fall back to default
	s.SetEngineConfig("zero_max", &SimpleEngineCacheConfig{
		Enabled: true,
		TTL:     5 * time.Minute,
		MaxSize: 0,
	})
	if size := s.GetMaxSizeForEngine("zero_max"); size != 1000 {
		t.Errorf("expected default 1000 for zero max size, got %d", size)
	}

	// Case insensitive
	if size := s.GetMaxSizeForEngine("SMALL_ENGINE"); size != 50 {
		t.Errorf("expected case-insensitive lookup, got %d", size)
	}
}

// ===== ConfigBasedCacheStrategy: GetAllEngineConfigs =====

func TestConfigBasedCacheStrategy_GetAllEngineConfigs(t *testing.T) {
	s := NewConfigBasedCacheStrategy(30 * time.Minute)

	// Empty initially
	configs := s.GetAllEngineConfigs()
	if len(configs) != 0 {
		t.Errorf("expected 0 configs, got %d", len(configs))
	}

	// Add configs
	s.SetEngineConfig("engine_a", &SimpleEngineCacheConfig{Enabled: true, TTL: 5 * time.Minute, MaxSize: 100})
	s.SetEngineConfig("engine_b", &SimpleEngineCacheConfig{Enabled: false, TTL: 10 * time.Minute, MaxSize: 200})

	configs = s.GetAllEngineConfigs()
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(configs))
	}

	// Verify the returned configs are copies
	delete(configs, "engine_a")
	configs2 := s.GetAllEngineConfigs()
	if len(configs2) != 2 {
		t.Error("returned map should be a copy")
	}
}

// ===== CacheStrategyManager: PrintStats =====

func TestCacheStrategyManager_PrintStats(t *testing.T) {
	m := NewCacheStrategyManager()
	// Just verify it doesn't panic with no strategies registered
	m.PrintStats()

	// Register a strategy and verify no panic
	m.RegisterStrategy("test", NewDefaultCacheStrategy(30*time.Minute))
	m.PrintStats()
}

// ===== DynamicCacheStrategy: GetCacheDuration with history =====

func TestDynamicCacheStrategy_GetCacheDuration_WithHistory(t *testing.T) {
	s := NewDynamicCacheStrategy(30*time.Minute, 5*time.Minute, 24*time.Hour)

	// First query: should return base duration
	d1 := s.GetCacheDuration("fofa", "test query", 1, 10)
	if d1 <= 0 {
		t.Error("expected positive default duration")
	}

	// Record some queries
	for i := 0; i < 5; i++ {
		s.RecordQuery("fofa", "test query", 1, 10, 200*time.Millisecond, true)
	}

	// After recording, duration should be influenced by history
	d2 := s.GetCacheDuration("fofa", "test query", 1, 10)
	if d2 <= 0 {
		t.Error("expected positive duration after recording")
	}
}

// ===== CacheStrategyStats =====

func TestCacheStrategyStats_Fields(t *testing.T) {
	stats := CacheStrategyStats{
		TotalQueries:    100,
		CacheHits:       50,
		CacheMisses:     50,
		AverageDuration: 1 * time.Second,
	}
	if stats.TotalQueries != 100 {
		t.Errorf("TotalQueries = %d, want 100", stats.TotalQueries)
	}
	if stats.CacheHits != 50 {
		t.Errorf("CacheHits = %d, want 50", stats.CacheHits)
	}
}

// ===== ConfigBasedCacheStrategy: SetEngineConfig directly =====

func TestConfigBasedCacheStrategy_SetEngineConfig(t *testing.T) {
	s := NewConfigBasedCacheStrategy(30 * time.Minute)
	cfg := &SimpleEngineCacheConfig{Enabled: true, TTL: 30 * time.Minute, MaxSize: 500}
	s.SetEngineConfig("my_engine", cfg)

	if !s.IsCacheEnabledForEngine("my_engine") {
		t.Error("expected engine enabled after SetEngineConfig")
	}
	if s.GetMaxSizeForEngine("my_engine") != 500 {
		t.Errorf("expected max size 500, got %d", s.GetMaxSizeForEngine("my_engine"))
	}
}

// ===== ConfigBasedCacheStrategy: SetEngineConfigFromMap edge cases =====

func TestConfigBasedCacheStrategy_SetEngineConfigFromMap_EdgeCases(t *testing.T) {
	s := NewConfigBasedCacheStrategy(30 * time.Minute)

	// Empty map
	s.SetEngineConfigFromMap(map[string]struct {
		Enabled bool
		TTL     int
		MaxSize int
	}{})
	// Should not panic and should use defaults

	// With various configs
	s.SetEngineConfigFromMap(map[string]struct {
		Enabled bool
		TTL     int
		MaxSize int
	}{
		"mixed": {Enabled: true, TTL: 600, MaxSize: 200},
	})

	if !s.IsCacheEnabledForEngine("mixed") {
		t.Error("expected mixed engine enabled")
	}
}

// ===== DynamicCacheStrategy boundary conditions =====

func TestDynamicCacheStrategy_BoundaryConditions(t *testing.T) {
	s := NewDynamicCacheStrategy(30*time.Minute, 5*time.Minute, 24*time.Hour)

	// Query with zero page/pageSize
	d := s.GetCacheDuration("engine", "query", 0, 0)
	if d <= 0 {
		t.Error("expected positive duration for zero page/pageSize")
	}

	// Record with zero duration
	s.RecordQuery("engine", "query", 0, 0, 0, true)

	// Record with failure
	s.RecordQuery("engine", "query", 1, 10, 5*time.Second, false)
}

// ===== SimpleEngineCacheConfig =====

func TestSimpleEngineCacheConfig_Defaults(t *testing.T) {
	cfg := &SimpleEngineCacheConfig{
		Enabled: true,
		TTL:     10 * time.Minute,
		MaxSize: 100,
	}
	if !cfg.IsEnabled() {
		t.Error("expected enabled")
	}
	if cfg.GetTTL() != 10*time.Minute {
		t.Errorf("expected TTL 10m, got %v", cfg.GetTTL())
	}
	if cfg.GetMaxSize() != 100 {
		t.Errorf("expected max size 100, got %d", cfg.GetMaxSize())
	}
}

// ===== CacheStrategyManager: Stats aggregation =====

func TestCacheStrategyManager_StatsAggregation(t *testing.T) {
	m := NewCacheStrategyManager()
	s := NewDefaultCacheStrategy(30 * time.Minute)
	s.RecordQuery("engine", "q", 1, 10, 100*time.Millisecond, true)

	m.RegisterStrategy("my_strat", s)

	// Record via manager's strategy
	strat := m.GetStrategy("my_strat")
	stats := strat.GetStats()
	if stats.TotalQueries != 1 {
		t.Errorf("expected 1 query, got %d", stats.TotalQueries)
	}
}

// ===== ValidateMinLength with unicode =====

func TestValidateMinLength_Unicode(t *testing.T) {
	v := NewValidator()
	v.ValidateMinLength("name", "中文", 2, "Name")
	if v.HasErrors() {
		t.Error("expected no error for 2-unicode-char string with min 2")
	}

	v2 := NewValidator()
	v2.ValidateMinLength("name", "中文", 3, "Name")
	if !v2.HasErrors() {
		t.Error("expected error for 2-unicode-char string with min 3")
	}
}

// ===== ValidateMaxLength with unicode =====

func TestValidateMaxLength_Unicode(t *testing.T) {
	v := NewValidator()
	v.ValidateMaxLength("name", "中文test", 6, "Name")
	if v.HasErrors() {
		t.Error("expected no error for 6 chars with max 6")
	}

	v2 := NewValidator()
	v2.ValidateMaxLength("name", "中文test!", 6, "Name")
	if !v2.HasErrors() {
		t.Error("expected error for 7 chars with max 6")
	}
}

// ===== ValidateEmail edge cases =====

func TestValidateEmail_EdgeCases(t *testing.T) {
	v := NewValidator()
	v.ValidateEmail("email", "user+tag@example.com")
	if v.HasErrors() {
		t.Error("expected valid email with + tag")
	}

	v2 := NewValidator()
	v2.ValidateEmail("email", "user@sub.example.com")
	if v2.HasErrors() {
		t.Error("expected valid email with subdomain")
	}
}

// ===== ValidateURL edge cases =====

func TestValidateURL_EdgeCases(t *testing.T) {
	v := NewValidator()
	v.ValidateURL("url", "https://user:pass@example.com/path?query=value#fragment")
	if v.HasErrors() {
		t.Error("expected valid full URL")
	}

	v2 := NewValidator()
	v2.ValidateURL("url", "ftp://ftp.example.com/file.txt")
	if v2.HasErrors() {
		t.Error("expected valid FTP URL")
	}
}

// ===== ValidateAPIKey boundary =====

func TestValidateAPIKey_Boundary(t *testing.T) {
	v := NewValidator()
	v.ValidateAPIKey("key", "abcdefghijklmnopqrstuvwxyz123456")
	if v.HasErrors() {
		t.Error("expected no error for exactly 32 chars")
	}

	v2 := NewValidator()
	v2.ValidateAPIKey("key", "abcdefghijklmnopqrstuvwxyz12345")
	if !v2.HasErrors() {
		t.Error("expected error for 31 chars")
	}
}

// ===== ValidateRequestSize edge cases =====

func TestValidateRequestSize_EdgeCases(t *testing.T) {
	exact := int64(10 * 1024 * 1024)
	if !ValidateRequestSize(exact, 10) {
		t.Error("expected true for exact size match")
	}

	if ValidateRequestSize(exact+1, 10) {
		t.Error("expected false for one byte over")
	}

	if ValidateRequestSize(1, 0) {
		t.Error("expected false for any content with zero limit")
	}
	if !ValidateRequestSize(0, 0) {
		t.Error("expected true for zero content with zero limit")
	}
}

// ===== ValidateContentType with charset =====

func TestValidateContentType_WithCharset(t *testing.T) {
	tests := []struct {
		ct   string
		exp  string
		want bool
	}{
		{"text/html; charset=utf-8", "text/html", true},
		{"text/plain", "text/html", false},
		{"APPLICATION/JSON", "application/json", true},
		{"application/json", "APPLICATION/JSON", true},
		{"multipart/form-data; boundary=abc", "multipart", true},
	}
	for _, tt := range tests {
		got := ValidateContentType(tt.ct, tt.exp)
		if got != tt.want {
			t.Errorf("ValidateContentType(%q, %q) = %v, want %v", tt.ct, tt.exp, got, tt.want)
		}
	}
}

// ===== Validator chained validations =====

func TestValidator_ChainedValidations(t *testing.T) {
	v := NewValidator()
	v.ValidateRequired("email", "", "Email")
	v.ValidateMinLength("email", "", 5, "Email")
	v.ValidateMaxLength("email", "very-long-email-address@example.com", 20, "Email")
	v.ValidateEmail("email", "not-an-email")

	errs := v.Errors()
	if len(errs) < 3 {
		t.Errorf("expected at least 3 errors, got %d", len(errs))
	}

	msg := v.ErrorMessage()
	if !strings.Contains(msg, "required") {
		t.Error("error message should contain 'required'")
	}
	if !strings.Contains(msg, "Invalid email") {
		t.Error("error message should contain 'Invalid email'")
	}
}
