package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- cookies.go tests ---

func TestParseCookieHeader_Single(t *testing.T) {
	cookies := ParseCookieHeader("session=abc123", ".example.com")
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	if cookies[0].Name != "session" || cookies[0].Value != "abc123" {
		t.Fatalf("unexpected cookie: %+v", cookies[0])
	}
	if cookies[0].Domain != ".example.com" {
		t.Fatalf("expected domain .example.com, got %s", cookies[0].Domain)
	}
}

func TestParseCookieHeader_Multiple(t *testing.T) {
	cookies := ParseCookieHeader("session=abc; token=xyz; id=123", ".test.com")
	if len(cookies) != 3 {
		t.Fatalf("expected 3 cookies, got %d", len(cookies))
	}
}

func TestParseCookieHeader_Empty(t *testing.T) {
	cookies := ParseCookieHeader("", "")
	if len(cookies) != 0 {
		t.Fatalf("expected 0 cookies, got %d", len(cookies))
	}
}

func TestParseCookieHeader_NoEquals(t *testing.T) {
	cookies := ParseCookieHeader("noseparator", "")
	if len(cookies) != 0 {
		t.Fatalf("expected 0 cookies for no-equals input, got %d", len(cookies))
	}
}

func TestParseCookieHeader_Whitespace(t *testing.T) {
	cookies := ParseCookieHeader("  ", "")
	if len(cookies) != 0 {
		t.Fatalf("expected 0 cookies for whitespace, got %d", len(cookies))
	}
}

func TestParseCookieHeader_SemicolonsOnly(t *testing.T) {
	cookies := ParseCookieHeader(";;", "")
	if len(cookies) != 0 {
		t.Fatalf("expected 0 cookies, got %d", len(cookies))
	}
}

func TestParseCookieJSON_Valid(t *testing.T) {
	jsonStr := `[{"name":"session","value":"abc","domain":"example.com"}]`
	cookies, err := ParseCookieJSON(jsonStr, ".default.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	if cookies[0].Name != "session" || cookies[0].Value != "abc" {
		t.Fatalf("unexpected cookie: %+v", cookies[0])
	}
}

func TestParseCookieJSON_Invalid(t *testing.T) {
	cookies, err := ParseCookieJSON("not json", "")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if len(cookies) != 0 {
		t.Fatalf("expected 0 cookies, got %d", len(cookies))
	}
}

func TestParseCookieJSON_Empty(t *testing.T) {
	cookies, err := ParseCookieJSON("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cookies) != 0 {
		t.Fatalf("expected 0 cookies for empty string, got %d", len(cookies))
	}
}

func TestParseCookieJSON_Array(t *testing.T) {
	jsonStr := `[{"name":"a","value":"1"},{"name":"b","value":"2"},{"name":"c","value":"3"}]`
	cookies, err := ParseCookieJSON(jsonStr, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cookies) != 3 {
		t.Fatalf("expected 3 cookies, got %d", len(cookies))
	}
}

func TestParseCookieJSON_WrongType(t *testing.T) {
	// JSON object instead of array
	cookies, err := ParseCookieJSON(`{"name":"session","value":"abc"}`, "")
	if err == nil {
		t.Fatal("expected error for non-array JSON")
	}
	if len(cookies) != 0 {
		t.Fatalf("expected 0 cookies, got %d", len(cookies))
	}
}

func TestDefaultCookieDomain_Fofa(t *testing.T) {
	assert.Equal(t, ".fofa.info", DefaultCookieDomain("fofa"))
}

func TestDefaultCookieDomain_Hunter(t *testing.T) {
	assert.Equal(t, ".hunter.qianxin.com", DefaultCookieDomain("hunter"))
}

func TestDefaultCookieDomain_Quake(t *testing.T) {
	assert.Equal(t, ".quake.360.cn", DefaultCookieDomain("quake"))
}

func TestDefaultCookieDomain_Zoomeye(t *testing.T) {
	assert.Equal(t, ".zoomeye.org", DefaultCookieDomain("zoomeye"))
}

func TestDefaultCookieDomain_Unknown(t *testing.T) {
	assert.Equal(t, "", DefaultCookieDomain("unknown"))
}

func TestDefaultCookieDomain_CaseInsensitive(t *testing.T) {
	assert.Equal(t, ".fofa.info", DefaultCookieDomain("FOFA"))
}

// --- Cache getter tests ---

func TestGetEngineCacheConfig_Enabled(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	mgr.applyDefaults(&cfg)
	cfg.Cache.Engines = map[string]EngineCacheConfig{
		"quake": {Enabled: true, TTL: 600, MaxSize: 1000},
	}
	mgr.config = &cfg

	cc := mgr.GetEngineCacheConfig("quake")
	assert.True(t, cc.Enabled)
	assert.Equal(t, 600, cc.TTL)
}

func TestGetEngineCacheConfig_Defaults(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	mgr.applyDefaults(&cfg)
	mgr.config = &cfg

	cc := mgr.GetEngineCacheConfig("unknown_engine")
	assert.True(t, cc.Enabled) // default is true for unknown
}

func TestGetAllEngineCacheConfigs(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	mgr.applyDefaults(&cfg)
	cfg.Cache.Engines = map[string]EngineCacheConfig{
		"quake":  {Enabled: true, TTL: 600},
		"fofa":   {Enabled: false, TTL: 300},
	}
	mgr.config = &cfg

	configs := mgr.GetAllEngineCacheConfigs()
	assert.Len(t, configs, 2)
}

func TestIsCacheEnabledForEngine(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	mgr.applyDefaults(&cfg)
	cfg.Cache.Engines = map[string]EngineCacheConfig{
		"quake": {Enabled: true},
	}
	mgr.config = &cfg

	assert.True(t, mgr.IsCacheEnabledForEngine("quake"))
	// Unknown engine returns default (Enabled: true)
	assert.True(t, mgr.IsCacheEnabledForEngine("unknown"))
}

func TestGetCacheTTLForEngine(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	mgr.applyDefaults(&cfg)
	cfg.Cache.Engines = map[string]EngineCacheConfig{
		"quake": {Enabled: true, TTL: 1200},
	}
	mgr.config = &cfg

	ttl := mgr.GetCacheTTLForEngine("quake")
	assert.Equal(t, 1200, ttl)

	// Unknown engine should return default
	ttl = mgr.GetCacheTTLForEngine("unknown")
	assert.Equal(t, 3600, ttl) // default from Cache.TTL
}

func TestGetCacheMaxSizeForEngine(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	mgr.applyDefaults(&cfg)
	cfg.Cache.Engines = map[string]EngineCacheConfig{
		"quake": {Enabled: true, MaxSize: 5000},
	}
	mgr.config = &cfg

	size := mgr.GetCacheMaxSizeForEngine("quake")
	assert.Equal(t, 5000, size)
}

func TestGetCacheBackend(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	mgr.applyDefaults(&cfg)
	cfg.Cache.Backend = "redis"
	mgr.config = &cfg

	assert.Equal(t, "redis", mgr.GetCacheBackend())
}

func TestGetCacheBackend_Default(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	mgr.applyDefaults(&cfg)
	mgr.config = &cfg

	assert.Equal(t, "memory", mgr.GetCacheBackend())
}

func TestGetRedisAddr(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	mgr.applyDefaults(&cfg)
	cfg.Cache.Redis.Addr = "redis:6380"
	mgr.config = &cfg

	assert.Equal(t, "redis:6380", mgr.GetRedisAddr())
}

func TestGetRedisPassword(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	mgr.applyDefaults(&cfg)
	cfg.Cache.Redis.Password = "secret"
	mgr.config = &cfg

	assert.Equal(t, "secret", mgr.GetRedisPassword())
}

func TestGetRedisDB(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	mgr.applyDefaults(&cfg)
	cfg.Cache.Redis.DB = 5
	mgr.config = &cfg

	assert.Equal(t, 5, mgr.GetRedisDB())
}

func TestGetRedisPrefix(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	mgr.applyDefaults(&cfg)
	cfg.Cache.Redis.Prefix = "myprefix:"
	mgr.config = &cfg

	assert.Equal(t, "myprefix:", mgr.GetRedisPrefix())
}

// --- normalizeProxyList tests ---

func TestNormalizeProxyList_Filters(t *testing.T) {
	input := []string{
		"http://proxy1:8080",
		"",
		"   ",
		"https://proxy2:443",
	}

	result := normalizeProxyList(input)
	assert.Len(t, result, 2)
	assert.Contains(t, result, "http://proxy1:8080")
	assert.Contains(t, result, "https://proxy2:443")
}

func TestNormalizeProxyList_NilSlice(t *testing.T) {
	result := normalizeProxyList(nil)
	assert.Empty(t, result)
}

func TestNormalizeProxyList_AllInvalid(t *testing.T) {
	input := []string{"", "  ", "\t"}
	result := normalizeProxyList(input)
	assert.Empty(t, result)
}

func TestNormalizeProxyList_Dedup(t *testing.T) {
	input := []string{
		"http://proxy1:8080",
		"http://proxy1:8080",
		"https://proxy2:443",
	}
	result := normalizeProxyList(input)
	assert.Len(t, result, 2)
}

func TestNormalizeProxyList_CommaSeparated(t *testing.T) {
	input := []string{"http://a:80,http://b:80"}
	result := normalizeProxyList(input)
	assert.Len(t, result, 2)
}

// --- GetConfig tests ---

func TestGetConfig_Nil(t *testing.T) {
	mgr := NewManager("test.yaml")
	cfg := mgr.GetConfig()
	assert.Nil(t, cfg)
}

func TestGetConfig_NonNil(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	mgr.applyDefaults(&cfg)
	mgr.config = &cfg

	got := mgr.GetConfig()
	assert.NotNil(t, got)
	assert.Equal(t, "info", got.Log.Level)
}

// --- Load with valid file ---

func TestLoad_WithValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpPath := filepath.Join(tmpDir, "config.yaml")

	// Create a minimal config file with env var substitution
	os.Setenv("TEST_API_KEY", "testkey123")
	defer os.Unsetenv("TEST_API_KEY")

	content := `
system:
  max_concurrent: 5
  log_level: debug
`
	err := os.WriteFile(tmpPath, []byte(content), 0644)
	assert.NoError(t, err)

	mgr := NewManager(tmpPath)
	err = mgr.Load()
	assert.NoError(t, err)
	assert.NotNil(t, mgr.config)
	assert.Equal(t, 5, mgr.config.System.MaxConcurrent)
}

func TestLoad_WithEnvVars(t *testing.T) {
	tmpDir := t.TempDir()
	tmpPath := filepath.Join(tmpDir, "config_env.yaml")

	os.Setenv("UNIMAP_LOG_LEVEL", "debug")
	defer os.Unsetenv("UNIMAP_LOG_LEVEL")

	// Use string field that will be resolved
	content := `
system:
  user_agent: "${UNIMAP_LOG_LEVEL}"
`
	err := os.WriteFile(tmpPath, []byte(content), 0644)
	assert.NoError(t, err)

	mgr := NewManager(tmpPath)
	err = mgr.Load()
	assert.NoError(t, err)
	assert.Equal(t, "debug", mgr.config.System.UserAgent)
}

// --- validate edge cases ---

func TestValidate_WebRateLimit(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	mgr.applyDefaults(&cfg)

	// Invalid: requests per window <= 0
	cfg.Web.RateLimit.RequestsPerWindow = 0
	err := mgr.validate(&cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requests_per_window")
}

func TestValidate_WebRateLimit_Window(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	mgr.applyDefaults(&cfg)

	cfg.Web.RateLimit.WindowSeconds = 0
	err := mgr.validate(&cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "window_seconds")
}

func TestValidate_RequestMaxBodyBytes(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	mgr.applyDefaults(&cfg)

	cfg.Web.RequestLimits.MaxBodyBytes = -1
	err := mgr.validate(&cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max_body_bytes")
}

func TestValidate_FofaEnabled(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	mgr.applyDefaults(&cfg)

	cfg.Engines.Fofa.Enabled = true
	cfg.Engines.Fofa.APIKey = ""
	cfg.Engines.Fofa.Email = ""
	cfg.Engines.Fofa.UseWebAPI = false
	err := mgr.validate(&cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api_key or email")
}

func TestApplyDefaults_NonZeroPreserved(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	cfg.System.MaxConcurrent = 50 // non-zero, should be preserved
	mgr.applyDefaults(&cfg)

	assert.Equal(t, 50, cfg.System.MaxConcurrent)
}
