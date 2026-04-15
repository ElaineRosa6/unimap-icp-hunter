package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// fileWatcher 文件监控器
type fileWatcher struct {
	filePath    string
	lastModTime time.Time
	lastSize    int64
}

// newFileWatcher 创建文件监控器
func newFileWatcher(filePath string) (*fileWatcher, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	return &fileWatcher{
		filePath:    filePath,
		lastModTime: info.ModTime(),
		lastSize:    info.Size(),
	}, nil
}

// Check 检查文件是否变化
func (w *fileWatcher) Check() (bool, error) {
	info, err := os.Stat(w.filePath)
	if err != nil {
		return false, fmt.Errorf("failed to stat file: %w", err)
	}

	if info.ModTime() != w.lastModTime || info.Size() != w.lastSize {
		w.lastModTime = info.ModTime()
		w.lastSize = info.Size()
		return true, nil
	}

	return false, nil
}

// Close 关闭监控器
func (w *fileWatcher) Close() {
	// 文件监控器不需要特殊清理
}

// calculateChecksum 计算配置校验和
func calculateChecksum(config *Config) (string, error) {
	// 将配置序列化为JSON
	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}

	// 计算SHA-256校验和
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// validateConfig 验证配置
func validateConfig(config *Config) error {
	// 验证系统配置
	if config.System.MaxConcurrent < 0 {
		return fmt.Errorf("invalid max concurrent: %d", config.System.MaxConcurrent)
	}

	if config.System.CacheMaxSize < 0 {
		return fmt.Errorf("invalid cache max size: %d", config.System.CacheMaxSize)
	}

	if config.System.CacheCleanupInterval < 0 {
		return fmt.Errorf("invalid cache cleanup interval: %d", config.System.CacheCleanupInterval)
	}

	if config.System.RetryAttempts < 0 {
		return fmt.Errorf("invalid retry attempts: %d", config.System.RetryAttempts)
	}

	// 验证缓存配置
	if config.Cache.Backend != "memory" && config.Cache.Backend != "redis" {
		return fmt.Errorf("invalid cache backend: %s", config.Cache.Backend)
	}

	// 验证Redis配置
	if config.Cache.Backend == "redis" {
		if config.Cache.Redis.Addr == "" {
			return fmt.Errorf("redis address is required")
		}
		if config.Cache.Redis.DB < 0 || config.Cache.Redis.DB > 15 {
			return fmt.Errorf("invalid redis DB: %d", config.Cache.Redis.DB)
		}
		if config.Cache.Redis.PoolSize < 0 {
			return fmt.Errorf("invalid redis pool size: %d", config.Cache.Redis.PoolSize)
		}
	}

	// 验证引擎配置
	// 检查每个引擎的配置
	engines := []struct {
		name       string
		enabled    bool
		apiKey     string
		requireKey bool
		timeout    int
		rateLimit  int
	}{
		{"quake", config.Engines.Quake.Enabled, config.Engines.Quake.APIKey, false, config.Engines.Quake.Timeout, config.Engines.Quake.QPS},
		{"zoomeye", config.Engines.Zoomeye.Enabled, config.Engines.Zoomeye.APIKey, false, config.Engines.Zoomeye.Timeout, config.Engines.Zoomeye.QPS},
		{"hunter", config.Engines.Hunter.Enabled, config.Engines.Hunter.APIKey, false, config.Engines.Hunter.Timeout, config.Engines.Hunter.QPS},
		{"fofa", config.Engines.Fofa.Enabled, config.Engines.Fofa.APIKey, config.Engines.Fofa.Enabled, config.Engines.Fofa.Timeout, config.Engines.Fofa.QPS},
		{"shodan", config.Engines.Shodan.Enabled, config.Engines.Shodan.APIKey, config.Engines.Shodan.Enabled, config.Engines.Shodan.Timeout, config.Engines.Shodan.QPS},
	}

	for _, engine := range engines {
		if engine.enabled && engine.requireKey && engine.apiKey == "" {
			return fmt.Errorf("API key required for engine: %s", engine.name)
		}
		if engine.timeout < 0 {
			return fmt.Errorf("invalid timeout for engine %s: %d", engine.name, engine.timeout)
		}
		if engine.rateLimit < 0 {
			return fmt.Errorf("invalid rate limit for engine %s: %d", engine.name, engine.rateLimit)
		}
	}

	return nil
}

// readConfigFile 读取配置文件
func readConfigFile(filePath string) ([]byte, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	return data, nil
}
