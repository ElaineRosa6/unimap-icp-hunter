package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManager_Load(t *testing.T) {
	// 创建临时配置文件
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	// 写入测试配置
	testConfig := `
engines:
  fofa:
    enabled: true
    email: test@example.com
    api_key: test_key
    base_url: https://api.fofa.info
    web_url: https://fofa.info
    qps: 1
    timeout: 30
  hunter:
    enabled: true
    api_key: test_key
    base_url: https://api.hunter.io
    qps: 1
    timeout: 30
  zoomeye:
    enabled: true
    api_key: test_key
    base_url: https://api.zoomeye.org
    qps: 1
    timeout: 30
  quake:
    enabled: true
    api_key: test_key
    base_url: https://api.360.cn
    qps: 1
    timeout: 30

system:
  max_concurrent: 10
  cache_ttl: 3600
  retry_attempts: 3
  user_agent: UniMap/1.0

log:
  level: info
  encoding: console
  file: ./logs/unimap.log
`

	err := os.WriteFile(configPath, []byte(testConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// 测试加载配置
	manager := NewManager(configPath)
	err = manager.Load()
	if err != nil {
		t.Errorf("Manager.Load() returned error: %v", err)
	}

	if !manager.IsValid() {
		t.Error("Manager.IsValid() returned false after loading valid config")
	}

	// 测试配置值
	config := manager.GetConfig()
	if config == nil {
		t.Fatal("Manager.GetConfig() returned nil")
	}

	if config.Engines.Fofa.Email != "test@example.com" {
		t.Errorf("Expected Fofa email to be 'test@example.com', got '%s'", config.Engines.Fofa.Email)
	}

	if config.Engines.Fofa.APIKey != "test_key" {
		t.Errorf("Expected Fofa API key to be 'test_key', got '%s'", config.Engines.Fofa.APIKey)
	}

	if config.Engines.Hunter.APIKey != "test_key" {
		t.Errorf("Expected Hunter API key to be 'test_key', got '%s'", config.Engines.Hunter.APIKey)
	}

	// 测试日志配置
	if config.Log.Level != "info" {
		t.Errorf("Expected log level to be 'info', got '%s'", config.Log.Level)
	}

	if config.Log.Encoding != "console" {
		t.Errorf("Expected log encoding to be 'console', got '%s'", config.Log.Encoding)
	}

	if config.Log.File != "./logs/unimap.log" {
		t.Errorf("Expected log file to be './logs/unimap.log', got '%s'", config.Log.File)
	}
}

func TestManager_Load_WithEnvVars(t *testing.T) {
	// 设置环境变量
	err := os.Setenv("FOFA_API_KEY", "env_fofa_key")
	if err != nil {
		t.Fatalf("Failed to set FOFA_API_KEY: %v", err)
	}
	defer os.Unsetenv("FOFA_API_KEY")

	err = os.Setenv("HUNTER_API_KEY", "env_hunter_key")
	if err != nil {
		t.Fatalf("Failed to set HUNTER_API_KEY: %v", err)
	}
	defer os.Unsetenv("HUNTER_API_KEY")

	// 创建临时配置文件，使用环境变量占位符
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	testConfig := `
engines:
  fofa:
    email: test@example.com
    api_key: $FOFA_API_KEY
    base_url: https://api.fofa.info
  hunter:
    api_key: ${HUNTER_API_KEY}
    base_url: https://api.hunter.io

system:
  user_agent: UniMap/1.0
`

	err = os.WriteFile(configPath, []byte(testConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// 测试加载配置
	manager := NewManager(configPath)
	err = manager.Load()
	if err != nil {
		t.Errorf("Manager.Load() returned error: %v", err)
	}

	// 测试环境变量解析
	config := manager.GetConfig()
	if config.Engines.Fofa.APIKey != "env_fofa_key" {
		t.Errorf("Expected Fofa API key to be 'env_fofa_key', got '%s'", config.Engines.Fofa.APIKey)
	}

	if config.Engines.Hunter.APIKey != "env_hunter_key" {
		t.Errorf("Expected Hunter API key to be 'env_hunter_key', got '%s'", config.Engines.Hunter.APIKey)
	}
}

func TestManager_Load_FileNotFound(t *testing.T) {
	// 测试加载不存在的配置文件
	nonExistentPath := filepath.Join(t.TempDir(), "non_existent_config.yaml")
	manager := NewManager(nonExistentPath)

	err := manager.Load()
	if err == nil {
		t.Error("Expected Manager.Load() to return error for non-existent file, but got nil")
	}

	// 即使文件不存在，也应该提供默认配置
	if !manager.IsValid() {
		t.Error("Manager.IsValid() returned false after loading non-existent config")
	}
}

func TestManager_ResolveEnv(t *testing.T) {
	manager := NewManager("")

	// 测试直接值
	if manager.ResolveEnv("direct_value") != "direct_value" {
		t.Error("ResolveEnv failed for direct value")
	}

	// 测试环境变量（不存在）
	if manager.ResolveEnv("$NON_EXISTENT_VAR") != "$NON_EXISTENT_VAR" {
		t.Error("ResolveEnv failed for non-existent env var")
	}

	// 测试环境变量（存在）
	err := os.Setenv("TEST_VAR", "test_value")
	if err != nil {
		t.Fatalf("Failed to set TEST_VAR: %v", err)
	}
	defer os.Unsetenv("TEST_VAR")

	if manager.ResolveEnv("$TEST_VAR") != "test_value" {
		t.Error("ResolveEnv failed for existing env var")
	}

	// 测试${}格式环境变量
	if manager.ResolveEnv("${TEST_VAR}") != "test_value" {
		t.Error("ResolveEnv failed for ${} format env var")
	}
}

func TestManager_IsValid(t *testing.T) {
	// 创建临时配置文件路径
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	manager := NewManager(configPath)

	// 初始状态应该无效
	if manager.IsValid() {
		t.Error("Manager.IsValid() returned true before loading config")
	}

	// 加载默认配置后应该有效
	err := manager.Load()
	// 即使文件不存在，Load()也应该返回错误但提供默认配置
	if err == nil {
		t.Error("Expected Manager.Load() to return error for non-existent file, but got nil")
	}

	if !manager.IsValid() {
		t.Error("Manager.IsValid() returned false after loading non-existent config")
	}
}
