package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load 加载配置文件
func (m *Manager) Load() error {
	// 读取配置文件
	data, err := os.ReadFile(m.path)
	if err != nil {
		var cfg Config
		m.applyDefaults(&cfg)
		m.resolveEnv(&cfg)
		m.SetConfig(&cfg)
		return fmt.Errorf("failed to read config file: %w", err)
	}

	candidate, err := m.parseConfig(data)
	if err != nil {
		var cfg Config
		m.applyDefaults(&cfg)
		m.resolveEnv(&cfg)
		m.SetConfig(&cfg)
		return err
	}
	m.SetConfig(candidate)
	return nil
}

// parseConfig normalizes and validates a candidate without publishing it.
// Startup and hot updates must interpret the same bytes identically.
func (m *Manager) parseConfig(data []byte) (*Config, error) {
	var candidate Config
	if err := yaml.Unmarshal(data, &candidate); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	m.applyDefaults(&candidate)
	m.resolveEnv(&candidate)
	DecryptNotifySecrets(&candidate)
	if err := m.validate(&candidate); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &candidate, nil
}

// resolveEnv 解析配置中的环境变量
func (m *Manager) resolveEnv(config *Config) {
	// 解析Quake配置
	config.Engines.Quake.APIKey = m.ResolveEnv(config.Engines.Quake.APIKey)
	config.Engines.Quake.BaseURL = m.ResolveEnv(config.Engines.Quake.BaseURL)

	// 解析ZoomEye配置
	config.Engines.Zoomeye.APIKey = m.ResolveEnv(config.Engines.Zoomeye.APIKey)
	config.Engines.Zoomeye.BaseURL = m.ResolveEnv(config.Engines.Zoomeye.BaseURL)

	// 解析Hunter配置
	config.Engines.Hunter.APIKey = m.ResolveEnv(config.Engines.Hunter.APIKey)
	config.Engines.Hunter.BaseURL = m.ResolveEnv(config.Engines.Hunter.BaseURL)

	// 解析FOFA配置
	config.Engines.Fofa.APIKey = m.ResolveEnv(config.Engines.Fofa.APIKey)
	config.Engines.Fofa.Email = m.ResolveEnv(config.Engines.Fofa.Email)
	config.Engines.Fofa.BaseURL = m.ResolveEnv(config.Engines.Fofa.BaseURL)
	config.Engines.Fofa.APIBaseURL = m.ResolveEnv(config.Engines.Fofa.APIBaseURL)
	config.Engines.Fofa.WebBaseURL = m.ResolveEnv(config.Engines.Fofa.WebBaseURL)

	// 解析Censys配置
	config.Engines.Censys.APIID = m.ResolveEnv(config.Engines.Censys.APIID)
	config.Engines.Censys.APISecret = m.ResolveEnv(config.Engines.Censys.APISecret)
	config.Engines.Censys.BaseURL = m.ResolveEnv(config.Engines.Censys.BaseURL)

	// 解析Shodan配置
	config.Engines.Shodan.APIKey = m.ResolveEnv(config.Engines.Shodan.APIKey)
	config.Engines.Shodan.BaseURL = m.ResolveEnv(config.Engines.Shodan.BaseURL)

	// 解析DayDayMap配置
	config.Engines.Daydaymap.APIKey = m.ResolveEnv(config.Engines.Daydaymap.APIKey)
	config.Engines.Daydaymap.BaseURL = m.ResolveEnv(config.Engines.Daydaymap.BaseURL)

	// 解析系统配置
	config.System.UserAgent = m.ResolveEnv(config.System.UserAgent)

	// 解析截图配置
	config.Screenshot.ChromePath = m.ResolveEnv(config.Screenshot.ChromePath)
	config.Screenshot.ProxyServer = m.ResolveEnv(config.Screenshot.ProxyServer)
	config.Screenshot.ChromeUserDataDir = m.ResolveEnv(config.Screenshot.ChromeUserDataDir)
	config.Screenshot.ChromeProfileDir = m.ResolveEnv(config.Screenshot.ChromeProfileDir)
	config.Screenshot.ChromeRemoteDebugURL = m.ResolveEnv(config.Screenshot.ChromeRemoteDebugURL)
	config.Screenshot.ChromeRemoteDebugAddress = m.ResolveEnv(config.Screenshot.ChromeRemoteDebugAddress)
	config.Screenshot.Engine = m.ResolveEnv(config.Screenshot.Engine)
	config.Screenshot.Mode = m.ResolveEnv(config.Screenshot.Mode)
	config.Screenshot.Priority = m.ResolveEnv(config.Screenshot.Priority)
	config.Screenshot.Extension.ListenAddr = m.ResolveEnv(config.Screenshot.Extension.ListenAddr)
	for i := range config.Network.ProxyPool.Proxies {
		config.Network.ProxyPool.Proxies[i] = m.ResolveEnv(config.Network.ProxyPool.Proxies[i])
	}
	config.Distributed.AdminToken = m.ResolveEnv(config.Distributed.AdminToken)
	for nodeID, token := range config.Distributed.NodeAuthTokens {
		config.Distributed.NodeAuthTokens[nodeID] = m.ResolveEnv(token)
	}
	config.Web.Auth.AdminToken = m.ResolveEnv(config.Web.Auth.AdminToken)
	config.Web.Auth.Username = m.ResolveEnv(config.Web.Auth.Username)
	config.Web.Auth.PasswordHash = m.ResolveEnv(config.Web.Auth.PasswordHash)

	// 解析旧版告警 Webhook 配置
	config.Alerting.Webhook.URL = m.ResolveEnv(config.Alerting.Webhook.URL)
	config.Alerting.Webhook.AuthToken = m.ResolveEnv(config.Alerting.Webhook.AuthToken)

	// 解析 ICP 配置
	config.ICP.BaseURL = m.ResolveEnv(config.ICP.BaseURL)
	config.ICP.APIKey = m.ResolveEnv(config.ICP.APIKey)

	// 解析缓存配置
	config.Cache.Backend = m.ResolveEnv(config.Cache.Backend)
	config.Cache.Redis.Addr = m.ResolveEnv(config.Cache.Redis.Addr)
	config.Cache.Redis.Password = m.ResolveEnv(config.Cache.Redis.Password)
	config.Cache.Redis.Prefix = m.ResolveEnv(config.Cache.Redis.Prefix)

	// 解析通知渠道环境变量
	if config.Notifications.FeishuApp != nil {
		config.Notifications.FeishuApp.AppID = m.ResolveEnv(config.Notifications.FeishuApp.AppID)
		config.Notifications.FeishuApp.AppSecret = m.ResolveEnv(config.Notifications.FeishuApp.AppSecret)
		config.Notifications.FeishuApp.ChatID = m.ResolveEnv(config.Notifications.FeishuApp.ChatID)
	}
	for i := range config.Notifications.Channels {
		config.Notifications.Channels[i].WebhookURL = m.ResolveEnv(config.Notifications.Channels[i].WebhookURL)
		config.Notifications.Channels[i].Secret = m.ResolveEnv(config.Notifications.Channels[i].Secret)
		config.Notifications.Channels[i].AppID = m.ResolveEnv(config.Notifications.Channels[i].AppID)
		config.Notifications.Channels[i].AppSecret = m.ResolveEnv(config.Notifications.Channels[i].AppSecret)
		config.Notifications.Channels[i].ChatID = m.ResolveEnv(config.Notifications.Channels[i].ChatID)
	}
}

// ResolveEnv 解析环境变量。
// 如果值是 $VAR 或 ${VAR} 格式（VAR 为合法环境变量名）但对应环境变量未设置，
// 返回空字符串（而非原始占位符），使下游能优雅跳过空配置（如未配置的 webhook URL）。
// 非占位符原样返回——例如以 $2a$10$ 开头的 bcrypt 哈希不会被误当作 $VAR 引用而清空。
func (m *Manager) ResolveEnv(value string) string {
	if name, ok := envVarName(value); ok {
		if envValue := os.Getenv(name); envValue != "" {
			return envValue
		}
		return "" // 环境变量未设置，返回空字符串
	}
	return value
}

// envVarName 提取 $VAR / ${VAR} 中的环境变量名；仅当剩余部分是合法
// 环境变量名（字母/数字/下划线，不以数字开头）时返回 ok=true。
func envVarName(value string) (string, bool) {
	var name string
	switch {
	case strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}"):
		name = strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}")
	case strings.HasPrefix(value, "$") && !strings.HasPrefix(value, "${"):
		name = strings.TrimPrefix(value, "$")
	default:
		return "", false
	}
	if name == "" {
		return "", false
	}
	for i, r := range name {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r == '_':
		case r >= '0' && r <= '9':
			if i == 0 {
				return "", false
			}
		default:
			return "", false
		}
	}
	return name, true
}
