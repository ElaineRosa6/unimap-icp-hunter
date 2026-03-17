package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 系统配置结构
type Config struct {
	Engines struct {
		Quake struct {
			Enabled bool     `yaml:"enabled"`
			APIKey  string   `yaml:"api_key"`
			BaseURL string   `yaml:"base_url"`
			QPS     int      `yaml:"qps"`
			Timeout int      `yaml:"timeout"`
			Cookies []Cookie `yaml:"cookies"`
		} `yaml:"quake"`
		Zoomeye struct {
			Enabled bool     `yaml:"enabled"`
			APIKey  string   `yaml:"api_key"`
			BaseURL string   `yaml:"base_url"`
			QPS     int      `yaml:"qps"`
			Timeout int      `yaml:"timeout"`
			Cookies []Cookie `yaml:"cookies"`
		} `yaml:"zoomeye"`
		Hunter struct {
			Enabled bool     `yaml:"enabled"`
			APIKey  string   `yaml:"api_key"`
			BaseURL string   `yaml:"base_url"`
			QPS     int      `yaml:"qps"`
			Timeout int      `yaml:"timeout"`
			Cookies []Cookie `yaml:"cookies"`
		} `yaml:"hunter"`
		Fofa struct {
			Enabled   bool     `yaml:"enabled"`
			APIKey    string   `yaml:"api_key"`
			Email     string   `yaml:"email"`
			BaseURL   string   `yaml:"base_url"`
			WebURL    string   `yaml:"web_url"`
			QPS       int      `yaml:"qps"`
			Timeout   int      `yaml:"timeout"`
			UseWebAPI bool     `yaml:"use_web_api"`
			Cookies   []Cookie `yaml:"cookies"`
		} `yaml:"fofa"`
		Shodan struct {
			Enabled bool   `yaml:"enabled"`
			APIKey  string `yaml:"api_key"`
			BaseURL string `yaml:"base_url"`
			QPS     int    `yaml:"qps"`
			Timeout int    `yaml:"timeout"`
		} `yaml:"shodan"`
	} `yaml:"engines"`

	// 系统配置
	System struct {
		MaxConcurrent int    `yaml:"max_concurrent"`
		CacheTTL      int    `yaml:"cache_ttl"`
		RetryAttempts int    `yaml:"retry_attempts"`
		UserAgent     string `yaml:"user_agent"`
	} `yaml:"system"`

	// 日志配置
	Log struct {
		Level    string `yaml:"level"`    // debug, info, warn, error, fatal
		Encoding string `yaml:"encoding"` // console, json
		File     string `yaml:"file"`     // 可选的日志文件路径
	} `yaml:"log"`

	// 截图配置
	Screenshot struct {
		Enabled              bool   `yaml:"enabled"`
		BaseDir              string `yaml:"base_dir"`
		ChromePath           string `yaml:"chrome_path"`
		ChromeUserDataDir    string `yaml:"chrome_user_data_dir"`
		ChromeProfileDir     string `yaml:"chrome_profile_dir"`
		ChromeRemoteDebugURL string `yaml:"chrome_remote_debug_url"`
		Headless             *bool  `yaml:"headless"`
		Timeout              int    `yaml:"timeout"`
		WindowWidth          int    `yaml:"window_width"`
		WindowHeight         int    `yaml:"window_height"`
		WaitTime             int    `yaml:"wait_time"`
		// 自动截图配置
		AutoCapture struct {
			Enabled              bool `yaml:"enabled"`
			CaptureSearchResults bool `yaml:"capture_search_results"`
			CaptureTargets       bool `yaml:"capture_targets"`
		} `yaml:"auto_capture"`
	} `yaml:"screenshot"`
}

// Cookie Cookie配置
type Cookie struct {
	Name     string `yaml:"name"`
	Value    string `yaml:"value"`
	Domain   string `yaml:"domain"`
	Path     string `yaml:"path"`
	HTTPOnly bool   `yaml:"http_only"`
	Secure   bool   `yaml:"secure"`
}

// Manager 配置管理器
type Manager struct {
	config *Config
	path   string
}

// NewManager 创建配置管理器
func NewManager(path string) *Manager {
	return &Manager{
		path: path,
	}
}

// Load 加载配置文件
func (m *Manager) Load() error {
	// 读取配置文件
	data, err := os.ReadFile(m.path)
	if err != nil {
		// 文件不存在/不可读时仍提供默认配置，避免上层直接崩溃
		var cfg Config
		m.applyDefaults(&cfg)
		m.resolveEnv(&cfg)
		m.config = &cfg
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// 解析配置
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		// 配置解析失败也提供默认配置（同时返回错误给上层提示）
		var cfg Config
		m.applyDefaults(&cfg)
		m.resolveEnv(&cfg)
		m.config = &cfg
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 应用默认值
	m.applyDefaults(&config)

	// 解析环境变量
	m.resolveEnv(&config)

	// 验证配置
	if err := m.validate(&config); err != nil {
		// 配置无效时保留默认值，避免上层直接崩溃
		var cfg Config
		m.applyDefaults(&cfg)
		m.resolveEnv(&cfg)
		m.config = &cfg
		return fmt.Errorf("invalid config: %w", err)
	}

	m.config = &config
	return nil
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
	config.Engines.Fofa.WebURL = m.ResolveEnv(config.Engines.Fofa.WebURL)

	// 解析系统配置
	config.System.UserAgent = m.ResolveEnv(config.System.UserAgent)

	// 解析截图配置
	config.Screenshot.ChromePath = m.ResolveEnv(config.Screenshot.ChromePath)
	config.Screenshot.ChromeUserDataDir = m.ResolveEnv(config.Screenshot.ChromeUserDataDir)
	config.Screenshot.ChromeProfileDir = m.ResolveEnv(config.Screenshot.ChromeProfileDir)
	config.Screenshot.ChromeRemoteDebugURL = m.ResolveEnv(config.Screenshot.ChromeRemoteDebugURL)
}

// ResolveEnv 解析环境变量
func (m *Manager) ResolveEnv(value string) string {
	// 检查是否包含环境变量
	if strings.HasPrefix(value, "$") {
		envName := strings.TrimPrefix(value, "$")
		if envValue := os.Getenv(envName); envValue != "" {
			return envValue
		}
	}
	// 检查是否包含${}格式的环境变量
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		envName := strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}")
		if envValue := os.Getenv(envName); envValue != "" {
			return envValue
		}
	}
	return value
}

// GetConfig 获取配置
func (m *Manager) GetConfig() *Config {
	return m.config
}

// applyDefaults 应用默认值
func (m *Manager) applyDefaults(config *Config) {
	// 默认引擎配置
	if config.Engines.Quake.BaseURL == "" {
		config.Engines.Quake.BaseURL = "https://quake.360.net/api"
	}
	if config.Engines.Quake.QPS == 0 {
		config.Engines.Quake.QPS = 5
	}
	if config.Engines.Quake.Timeout == 0 {
		config.Engines.Quake.Timeout = 30
	}

	if config.Engines.Zoomeye.BaseURL == "" {
		config.Engines.Zoomeye.BaseURL = "https://api.zoomeye.org"
	}
	if config.Engines.Zoomeye.QPS == 0 {
		config.Engines.Zoomeye.QPS = 3
	}
	if config.Engines.Zoomeye.Timeout == 0 {
		config.Engines.Zoomeye.Timeout = 30
	}

	if config.Engines.Hunter.BaseURL == "" {
		config.Engines.Hunter.BaseURL = "https://hunter.qianxin.com"
	}
	if config.Engines.Hunter.QPS == 0 {
		config.Engines.Hunter.QPS = 5
	}
	if config.Engines.Hunter.Timeout == 0 {
		config.Engines.Hunter.Timeout = 30
	}

	if config.Engines.Fofa.BaseURL == "" {
		config.Engines.Fofa.BaseURL = "https://fofa.info"
	}
	if config.Engines.Fofa.WebURL == "" {
		config.Engines.Fofa.WebURL = "https://fofa.info"
	}
	if config.Engines.Fofa.QPS == 0 {
		config.Engines.Fofa.QPS = 3
	}
	if config.Engines.Fofa.Timeout == 0 {
		config.Engines.Fofa.Timeout = 30
	}

	if config.Engines.Shodan.BaseURL == "" {
		config.Engines.Shodan.BaseURL = "https://api.shodan.io"
	}
	if config.Engines.Shodan.QPS == 0 {
		config.Engines.Shodan.QPS = 1
	}
	if config.Engines.Shodan.Timeout == 0 {
		config.Engines.Shodan.Timeout = 30
	}

	// 默认系统配置
	if config.System.MaxConcurrent == 0 {
		config.System.MaxConcurrent = 10
	}
	if config.System.CacheTTL == 0 {
		config.System.CacheTTL = 3600
	}
	if config.System.RetryAttempts == 0 {
		config.System.RetryAttempts = 3
	}
	if config.System.UserAgent == "" {
		config.System.UserAgent = "UniMap-ICP-Hunter/1.0"
	}

	// 默认日志配置
	if config.Log.Level == "" {
		config.Log.Level = "info"
	}
	if config.Log.Encoding == "" {
		config.Log.Encoding = "console"
	}

	// 默认截图配置
	if config.Screenshot.Headless == nil {
		defaultHeadless := true
		config.Screenshot.Headless = &defaultHeadless
	}
	// Log.File 默认为空，表示只输出到标准输出

	// 默认截图配置
	if config.Screenshot.BaseDir == "" {
		config.Screenshot.BaseDir = "./screenshots"
	}
	if config.Screenshot.Timeout == 0 {
		config.Screenshot.Timeout = 30
	}
	if config.Screenshot.WindowWidth == 0 {
		config.Screenshot.WindowWidth = 1365
	}
	if config.Screenshot.WindowHeight == 0 {
		config.Screenshot.WindowHeight = 768
	}
	if config.Screenshot.WaitTime == 0 {
		config.Screenshot.WaitTime = 500
	}
}

// validate 验证配置有效性
func (m *Manager) validate(config *Config) error {
	// 验证引擎配置
	if config.Engines.Quake.Enabled {
		if config.Engines.Quake.BaseURL == "" {
			return fmt.Errorf("quake engine enabled but base_url not set")
		}
		if config.Engines.Quake.QPS <= 0 {
			return fmt.Errorf("quake engine qps must be greater than 0")
		}
		if config.Engines.Quake.Timeout <= 0 {
			return fmt.Errorf("quake engine timeout must be greater than 0")
		}
	}

	if config.Engines.Zoomeye.Enabled {
		if config.Engines.Zoomeye.BaseURL == "" {
			return fmt.Errorf("zoomeye engine enabled but base_url not set")
		}
		if config.Engines.Zoomeye.QPS <= 0 {
			return fmt.Errorf("zoomeye engine qps must be greater than 0")
		}
		if config.Engines.Zoomeye.Timeout <= 0 {
			return fmt.Errorf("zoomeye engine timeout must be greater than 0")
		}
	}

	if config.Engines.Hunter.Enabled {
		if config.Engines.Hunter.BaseURL == "" {
			return fmt.Errorf("hunter engine enabled but base_url not set")
		}
		if config.Engines.Hunter.QPS <= 0 {
			return fmt.Errorf("hunter engine qps must be greater than 0")
		}
		if config.Engines.Hunter.Timeout <= 0 {
			return fmt.Errorf("hunter engine timeout must be greater than 0")
		}
	}

	if config.Engines.Fofa.Enabled {
		if !config.Engines.Fofa.UseWebAPI {
			if config.Engines.Fofa.APIKey == "" || config.Engines.Fofa.Email == "" {
				return fmt.Errorf("fofa engine enabled but api_key or email not set")
			}
		}
		if config.Engines.Fofa.BaseURL == "" {
			return fmt.Errorf("fofa engine enabled but base_url not set")
		}
		if config.Engines.Fofa.WebURL == "" {
			return fmt.Errorf("fofa engine enabled but web_url not set")
		}
		if config.Engines.Fofa.QPS <= 0 {
			return fmt.Errorf("fofa engine qps must be greater than 0")
		}
		if config.Engines.Fofa.Timeout <= 0 {
			return fmt.Errorf("fofa engine timeout must be greater than 0")
		}
	}

	// 验证系统配置
	if config.System.MaxConcurrent <= 0 {
		return fmt.Errorf("system max_concurrent must be greater than 0")
	}
	if config.System.CacheTTL <= 0 {
		return fmt.Errorf("system cache_ttl must be greater than 0")
	}
	if config.System.RetryAttempts < 0 {
		return fmt.Errorf("system retry_attempts must be greater than or equal to 0")
	}
	if config.System.UserAgent == "" {
		return fmt.Errorf("system user_agent must be set")
	}

	return nil
}

// IsValid 检查配置是否有效
func (m *Manager) IsValid() bool {
	return m.config != nil
}

// GetEngineConfig 获取引擎配置
func (m *Manager) GetEngineConfig(name string) (interface{}, error) {
	if !m.IsValid() {
		return nil, fmt.Errorf("config not loaded")
	}

	switch strings.ToLower(name) {
	case "quake":
		return &m.config.Engines.Quake, nil
	case "zoomeye":
		return &m.config.Engines.Zoomeye, nil
	case "hunter":
		return &m.config.Engines.Hunter, nil
	case "fofa":
		return &m.config.Engines.Fofa, nil
	default:
		return nil, fmt.Errorf("unknown engine: %s", name)
	}
}

// Save 保存配置文件
func (m *Manager) Save() error {
	if m.config == nil {
		return fmt.Errorf("config is nil")
	}

	data, err := yaml.Marshal(m.config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// 确保目录存在
	dir := filepath.Dir(m.path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	if err := os.WriteFile(m.path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}
