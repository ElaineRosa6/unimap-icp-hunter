package config

import (
	"fmt"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/logger"
	"gopkg.in/yaml.v3"
)

// HotUpdateManager 配置热更新管理器
type HotUpdateManager struct {
	configPath    string
	configManager *Manager
	watcher       *fileWatcher
	mutex         sync.RWMutex
	configHistory []ConfigVersion
	maxHistory    int
	running       bool
	stopChan      chan struct{}
}

// ConfigVersion 配置版本信息
type ConfigVersion struct {
	Version    int
	Config     *Config
	Timestamp  time.Time
	Checksum   string
	ChangeType string // "create", "update", "rollback"
}

// HotUpdateConfig 热更新配置
type HotUpdateConfig struct {
	Enabled         bool
	CheckInterval   time.Duration
	MaxHistorySize  int
	AutoRollback    bool
	RollbackTimeout time.Duration
}

// NewHotUpdateManager 创建热更新管理器
func NewHotUpdateManager(configPath string, configManager *Manager) *HotUpdateManager {
	manager := &HotUpdateManager{
		configPath:    configPath,
		configManager: configManager,
		maxHistory:    10,
		configHistory: make([]ConfigVersion, 0),
		stopChan:      make(chan struct{}),
	}

	return manager
}

// Start 启动热更新监控
func (h *HotUpdateManager) Start(cfg HotUpdateConfig) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.running {
		return fmt.Errorf("hot update manager already running")
	}

	if !cfg.Enabled {
		logger.Info("Hot update disabled")
		return nil
	}

	// 设置默认值
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 30 * time.Second
	}
	if cfg.MaxHistorySize > 0 {
		h.maxHistory = cfg.MaxHistorySize
	}
	if cfg.RollbackTimeout <= 0 {
		cfg.RollbackTimeout = 5 * time.Minute
	}

	// 创建文件监控器
	watcher, err := newFileWatcher(h.configPath)
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}
	h.watcher = watcher

	// 记录当前配置版本
	currentConfig := h.configManager.GetConfig()
	checksum, err := calculateChecksum(currentConfig)
	if err == nil {
		h.addConfigVersion(currentConfig, checksum, "create")
	}

	// 启动监控协程
	h.running = true
	go h.monitorLoop(cfg)

	logger.Infof("Hot update manager started with check interval: %v", cfg.CheckInterval)
	return nil
}

// Stop 停止热更新监控
func (h *HotUpdateManager) Stop() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if !h.running {
		return
	}

	h.running = false
	close(h.stopChan)

	if h.watcher != nil {
		h.watcher.Close()
		h.watcher = nil
	}

	logger.Info("Hot update manager stopped")
}

// monitorLoop 监控循环
func (h *HotUpdateManager) monitorLoop(cfg HotUpdateConfig) {
	ticker := time.NewTicker(cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.checkConfigChanges(cfg)
		case <-h.stopChan:
			return
		}
	}
}

// checkConfigChanges 检查配置变化
func (h *HotUpdateManager) checkConfigChanges(cfg HotUpdateConfig) {
	// 重新加载配置文件
	newConfig := &Config{}
	data, err := readConfigFile(h.configPath)
	if err != nil {
		logger.Errorf("Failed to read config file for hot update: %v", err)
		return
	}

	if err := yaml.Unmarshal(data, newConfig); err != nil {
		logger.Errorf("Failed to parse config file for hot update: %v", err)
		return
	}

	// 验证配置
	if err := validateConfig(newConfig); err != nil {
		logger.Errorf("Invalid config file for hot update: %v", err)
		return
	}

	// 计算校验和
	newChecksum, err := calculateChecksum(newConfig)
	if err != nil {
		logger.Errorf("Failed to calculate config checksum: %v", err)
		return
	}

	// 检查是否有变化
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if len(h.configHistory) > 0 {
		lastVersion := h.configHistory[len(h.configHistory)-1]
		if newChecksum == lastVersion.Checksum {
			return // 配置未变化
		}
	}

	// 更新配置
	oldConfig := h.configManager.GetConfig()
	h.configManager.config = newConfig

	// 记录新版本
	h.addConfigVersion(newConfig, newChecksum, "update")

	logger.Infof("Config hot update completed: version=%d", len(h.configHistory))

	// 如果启用自动回滚，启动回滚定时器
	if cfg.AutoRollback {
		go h.scheduleRollback(cfg.RollbackTimeout, oldConfig)
	}
}

// addConfigVersion 添加配置版本
func (h *HotUpdateManager) addConfigVersion(config *Config, checksum, changeType string) {
	cloned := config.Clone()
	if cloned == nil {
		return
	}
	version := ConfigVersion{
		Version:    len(h.configHistory) + 1,
		Config:     cloned,
		Timestamp:  time.Now(),
		Checksum:   checksum,
		ChangeType: changeType,
	}

	h.configHistory = append(h.configHistory, version)

	// 限制历史记录数量
	if len(h.configHistory) > h.maxHistory {
		h.configHistory = h.configHistory[len(h.configHistory)-h.maxHistory:]
	}
}

// scheduleRollback 安排回滚
func (h *HotUpdateManager) scheduleRollback(timeout time.Duration, rollbackConfig *Config) {
	time.Sleep(timeout)

	h.mutex.Lock()
	defer h.mutex.Unlock()

	if !h.running {
		return
	}

	// 检查是否需要回滚（如果期间有新的更新，则不回滚）
	if len(h.configHistory) > 1 {
		lastVersion := h.configHistory[len(h.configHistory)-1]
		if lastVersion.ChangeType == "update" {
			// 执行回滚
			h.configManager.config = rollbackConfig
			checksum, err := calculateChecksum(rollbackConfig)
			if err == nil {
				h.addConfigVersion(rollbackConfig, checksum, "rollback")
			}
			logger.Warn("Config auto rollback executed")
		}
	}
}

// GetConfigHistory 获取配置历史
func (h *HotUpdateManager) GetConfigHistory() []ConfigVersion {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	history := make([]ConfigVersion, len(h.configHistory))
	copy(history, h.configHistory)
	return history
}

// Rollback 手动回滚到指定版本
func (h *HotUpdateManager) Rollback(version int) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if version <= 0 || version > len(h.configHistory) {
		return fmt.Errorf("invalid version: %d", version)
	}

	targetVersion := h.configHistory[version-1]
	h.configManager.config = targetVersion.Config

	checksum, err := calculateChecksum(targetVersion.Config)
	if err == nil {
		h.addConfigVersion(targetVersion.Config, checksum, "rollback")
	}

	logger.Infof("Config rolled back to version: %d", version)
	return nil
}

// GetCurrentVersion 获取当前版本
func (h *HotUpdateManager) GetCurrentVersion() int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return len(h.configHistory)
}
