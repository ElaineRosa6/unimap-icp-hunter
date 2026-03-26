package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewManager(t *testing.T) {
	mgr := NewManager("test.yaml")
	assert.NotNil(t, mgr)
	assert.Equal(t, "test.yaml", mgr.path)
}

func TestResolveEnv(t *testing.T) {
	mgr := NewManager("test.yaml")
	
	// Test $VAR format
	os.Setenv("TEST_VAR", "test_value")
	defer os.Unsetenv("TEST_VAR")
	
	result := mgr.ResolveEnv("$TEST_VAR")
	assert.Equal(t, "test_value", result)
	
	// Test ${VAR} format
	result = mgr.ResolveEnv("${TEST_VAR}")
	assert.Equal(t, "test_value", result)
	
	// Test non-existent env var
	result = mgr.ResolveEnv("$NON_EXISTENT")
	assert.Equal(t, "$NON_EXISTENT", result)
	
	// Test empty string
	result = mgr.ResolveEnv("")
	assert.Equal(t, "", result)
	
	// Test regular string
	result = mgr.ResolveEnv("regular_string")
	assert.Equal(t, "regular_string", result)
}

func TestApplyDefaults(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	
	mgr.applyDefaults(&cfg)
	
	// Test default engine configurations
	assert.Equal(t, "https://quake.360.net/api", cfg.Engines.Quake.BaseURL)
	assert.Equal(t, 5, cfg.Engines.Quake.QPS)
	assert.Equal(t, 30, cfg.Engines.Quake.Timeout)
	
	assert.Equal(t, "https://api.zoomeye.org", cfg.Engines.Zoomeye.BaseURL)
	assert.Equal(t, 3, cfg.Engines.Zoomeye.QPS)
	assert.Equal(t, 30, cfg.Engines.Zoomeye.Timeout)
	
	assert.Equal(t, "https://hunter.qianxin.com", cfg.Engines.Hunter.BaseURL)
	assert.Equal(t, 5, cfg.Engines.Hunter.QPS)
	assert.Equal(t, 30, cfg.Engines.Hunter.Timeout)
	
	assert.Equal(t, "https://fofa.info", cfg.Engines.Fofa.BaseURL)
	assert.Equal(t, "https://fofa.info", cfg.Engines.Fofa.WebURL)
	assert.Equal(t, 3, cfg.Engines.Fofa.QPS)
	assert.Equal(t, 30, cfg.Engines.Fofa.Timeout)
	
	assert.Equal(t, "https://api.shodan.io", cfg.Engines.Shodan.BaseURL)
	assert.Equal(t, 1, cfg.Engines.Shodan.QPS)
	assert.Equal(t, 30, cfg.Engines.Shodan.Timeout)
	
	// Test default system configurations
	assert.Equal(t, 10, cfg.System.MaxConcurrent)
	assert.Equal(t, 3600, cfg.System.CacheTTL)
	assert.Equal(t, 1000, cfg.System.CacheMaxSize)
	assert.Equal(t, 300, cfg.System.CacheCleanupInterval)
	assert.Equal(t, 3, cfg.System.RetryAttempts)
	assert.Equal(t, "UniMap-ICP-Hunter/1.0", cfg.System.UserAgent)
	
	// Test default log configurations
	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, "console", cfg.Log.Encoding)
	
	// Test default screenshot configurations
	assert.Equal(t, "./screenshots", cfg.Screenshot.BaseDir)
	assert.Equal(t, 30, cfg.Screenshot.Timeout)
	assert.Equal(t, 1365, cfg.Screenshot.WindowWidth)
	assert.Equal(t, 768, cfg.Screenshot.WindowHeight)
	assert.Equal(t, 500, cfg.Screenshot.WaitTime)
	
	// Test default web configurations
	assert.Equal(t, []string{"http://localhost:8448", "http://127.0.0.1:8448"}, cfg.Web.CORS.AllowedOrigins)
	assert.Equal(t, []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}, cfg.Web.CORS.AllowedMethods)
	assert.Equal(t, []string{"Content-Type", "Authorization", "X-Requested-With", "X-WebSocket-Token"}, cfg.Web.CORS.AllowedHeaders)
	assert.Equal(t, 600, cfg.Web.CORS.MaxAge)
	
	assert.Equal(t, 60, cfg.Web.RateLimit.RequestsPerWindow)
	assert.Equal(t, 60, cfg.Web.RateLimit.WindowSeconds)
	
	assert.Equal(t, int64(10*1024*1024), cfg.Web.RequestLimits.MaxBodyBytes)
	assert.Equal(t, int64(10*1024*1024), cfg.Web.RequestLimits.MaxMultipartMemory)
	
	// Test default cache configurations
	assert.Equal(t, "memory", cfg.Cache.Backend)
	assert.Equal(t, "127.0.0.1:6379", cfg.Cache.Redis.Addr)
	assert.Equal(t, "unimap:", cfg.Cache.Redis.Prefix)
}

func TestValidate(t *testing.T) {
	mgr := NewManager("test.yaml")
	
	// Test valid configuration
	var validCfg Config
	mgr.applyDefaults(&validCfg)
	err := mgr.validate(&validCfg)
	assert.NoError(t, err)
	
	// Test invalid configuration - system max_concurrent <= 0
	invalidCfg := validCfg
	invalidCfg.System.MaxConcurrent = 0
	err = mgr.validate(&invalidCfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "system max_concurrent must be greater than 0")
	
	// Test invalid configuration - cache backend
	invalidCfg = validCfg
	invalidCfg.Cache.Backend = "invalid_backend"
	err = mgr.validate(&invalidCfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cache backend must be one of: memory, redis")
	
	// Test invalid configuration - quake QPS <= 0 (must be enabled first)
	invalidCfg = validCfg
	invalidCfg.Engines.Quake.Enabled = true
	invalidCfg.Engines.Quake.QPS = 0
	err = mgr.validate(&invalidCfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "quake engine qps must be greater than 0")
}

func TestGetEngineConfig(t *testing.T) {
	mgr := NewManager("test.yaml")
	var cfg Config
	mgr.applyDefaults(&cfg)
	mgr.config = &cfg
	
	// Test getting quake config
	quakeCfg, err := mgr.GetEngineConfig("quake")
	assert.NoError(t, err)
	assert.NotNil(t, quakeCfg)
	
	// Test getting zoomeye config
	zoomeyeCfg, err := mgr.GetEngineConfig("zoomeye")
	assert.NoError(t, err)
	assert.NotNil(t, zoomeyeCfg)
	
	// Test getting hunter config
	hunterCfg, err := mgr.GetEngineConfig("hunter")
	assert.NoError(t, err)
	assert.NotNil(t, hunterCfg)
	
	// Test getting fofa config
	fofaCfg, err := mgr.GetEngineConfig("fofa")
	assert.NoError(t, err)
	assert.NotNil(t, fofaCfg)
	
	// Test getting unknown engine
	unknownCfg, err := mgr.GetEngineConfig("unknown")
	assert.Error(t, err)
	assert.Nil(t, unknownCfg)
	assert.Contains(t, err.Error(), "unknown engine")
}

func TestIsValid(t *testing.T) {
	mgr := NewManager("test.yaml")
	
	// Test invalid config
	assert.False(t, mgr.IsValid())
	
	// Test valid config
	var cfg Config
	mgr.config = &cfg
	assert.True(t, mgr.IsValid())
}

func TestLoadWithNonExistentFile(t *testing.T) {
	mgr := NewManager("non_existent_config.yaml")
	err := mgr.Load()
	
	// Should return error but still have default config
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read config file")
	assert.NotNil(t, mgr.config)
}

func TestSave(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "config_test_*.yaml")
	assert.NoError(t, err)
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)
	
	mgr := NewManager(tmpPath)
	var cfg Config
	mgr.applyDefaults(&cfg)
	mgr.config = &cfg
	
	// Test save
	err = mgr.Save()
	assert.NoError(t, err)
	
	// Verify file exists and can be loaded
	data, err := os.ReadFile(tmpPath)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
	
	// Test save with nil config
	mgr.config = nil
	err = mgr.Save()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config is nil")
}

func TestResolveEnvWithComplexValues(t *testing.T) {
	mgr := NewManager("test.yaml")
	
	// Set up test environment variables
	os.Setenv("API_KEY", "secret123")
	os.Setenv("DB_PASSWORD", "password456")
	defer os.Unsetenv("API_KEY")
	defer os.Unsetenv("DB_PASSWORD")
	
	// Test individual env vars (ResolveEnv only handles standalone env vars)
	testCases := []struct {
		input    string
		expected string
	}{
		{"${API_KEY}", "secret123"},
		{"$DB_PASSWORD", "password456"},
		{"regular_text", "regular_text"},
		{"${NON_EXISTENT}", "${NON_EXISTENT}"},
		{"$NON_EXISTENT", "$NON_EXISTENT"},
	}
	
	for _, tc := range testCases {
		result := mgr.ResolveEnv(tc.input)
		assert.Equal(t, tc.expected, result, "Failed for input: %s", tc.input)
	}
}
