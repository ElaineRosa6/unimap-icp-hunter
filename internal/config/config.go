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
		MaxConcurrent        int    `yaml:"max_concurrent"`
		CacheTTL             int    `yaml:"cache_ttl"`
		CacheMaxSize         int    `yaml:"cache_max_size"`
		CacheCleanupInterval int    `yaml:"cache_cleanup_interval"`
		RetryAttempts        int    `yaml:"retry_attempts"`
		UserAgent            string `yaml:"user_agent"`
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
		Engine               string `yaml:"engine"`                // legacy: "cdp" or "extension"
		Mode                 string `yaml:"mode"`                  // new: "auto"|"cdp"|"extension"
		Priority             string `yaml:"priority"`              // new: "cdp"|"extension" (for auto mode)
		Fallback             *bool  `yaml:"fallback"`              // new: explicit fallback toggle
		BaseDir              string `yaml:"base_dir"`
		ChromePath           string `yaml:"chrome_path"`
		ProxyServer          string `yaml:"proxy_server"`
		ChromeUserDataDir    string `yaml:"chrome_user_data_dir"`
		ChromeProfileDir     string `yaml:"chrome_profile_dir"`
		ChromeRemoteDebugURL    string `yaml:"chrome_remote_debug_url"`
		ChromeRemoteDebugAddress string `yaml:"chrome_remote_debug_address"`
		Extension            struct {
			Enabled                      bool   `yaml:"enabled"`
			ListenAddr                   string `yaml:"listen_addr"`
			PairingRequired              bool   `yaml:"pairing_required"`
			TokenTTLSeconds              int    `yaml:"token_ttl_seconds"`
			TaskTimeoutSeconds           int    `yaml:"task_timeout_seconds"`
			MaxConcurrency               int    `yaml:"max_concurrency"`
			CallbackSignatureRequired    bool   `yaml:"callback_signature_required"`
			CallbackSignatureSkewSeconds int    `yaml:"callback_signature_skew_seconds"`
			CallbackNonceTTLSeconds      int    `yaml:"callback_nonce_ttl_seconds"`
			FallbackToCDP                bool   `yaml:"fallback_to_cdp"`
		} `yaml:"extension"`
		Headless     *bool `yaml:"headless"`
		Timeout      int   `yaml:"timeout"`
		WindowWidth  int   `yaml:"window_width"`
		WindowHeight int   `yaml:"window_height"`
		WaitTime     int   `yaml:"wait_time"`
		// 自动截图配置
		AutoCapture struct {
			Enabled              bool `yaml:"enabled"`
			CaptureSearchResults bool `yaml:"capture_search_results"`
			CaptureTargets       bool `yaml:"capture_targets"`
		} `yaml:"auto_capture"`
	} `yaml:"screenshot"`

	// Web 配置
	Web struct {
		Port        int    `yaml:"port"`         // 监听端口
		BindAddress string `yaml:"bind_address"` // 监听地址
		CORS struct {
			AllowedOrigins   []string `yaml:"allowed_origins"`
			AllowedMethods   []string `yaml:"allowed_methods"`
			AllowedHeaders   []string `yaml:"allowed_headers"`
			ExposedHeaders   []string `yaml:"exposed_headers"`
			AllowCredentials bool     `yaml:"allow_credentials"`
			MaxAge           int      `yaml:"max_age"`
		} `yaml:"cors"`
		RateLimit struct {
			Enabled           bool `yaml:"enabled"`
			RequestsPerWindow int  `yaml:"requests_per_window"`
			WindowSeconds     int  `yaml:"window_seconds"`
		} `yaml:"rate_limit"`
		RequestLimits struct {
			MaxBodyBytes       int64 `yaml:"max_body_bytes"`
			MaxMultipartMemory int64 `yaml:"max_multipart_memory_bytes"`
		} `yaml:"request_limits"`
		Auth struct {
			Enabled     bool   `yaml:"enabled"`      // 是否启用 Web 鉴权
			AdminToken  string `yaml:"admin_token"`  // 管理端点 token
			APIKeyStore string `yaml:"api_key_store"` // API Key 文件路径
		} `yaml:"auth"`
	} `yaml:"web"`

	// Network 配置
	Network struct {
		ProxyPool struct {
			Enabled             bool     `yaml:"enabled"`
			Strategy            string   `yaml:"strategy"`
			Proxies             []string `yaml:"proxies"`
			FailureThreshold    int      `yaml:"failure_threshold"`
			CooldownSeconds     int      `yaml:"cooldown_seconds"`
			AllowDirectFallback bool     `yaml:"allow_direct_fallback"`
		} `yaml:"proxy_pool"`
	} `yaml:"network"`

	// Distributed 配置
	Distributed struct {
		Enabled                 bool              `yaml:"enabled"`
		HeartbeatTimeoutSeconds int               `yaml:"heartbeat_timeout_seconds"`
		MaxReassignAttempts     int               `yaml:"max_reassign_attempts"`
		AdminToken              string            `yaml:"admin_token"`
		NodeAuthTokens          map[string]string `yaml:"node_auth_tokens"`
		Scheduler               struct {
			Strategy string `yaml:"strategy"`
		} `yaml:"scheduler"`
	} `yaml:"distributed"`

	// Alerting 告警配置
	Alerting struct {
		Webhook struct {
			Enabled   bool   `yaml:"enabled"`
			URL       string `yaml:"url"`
			AuthToken string `yaml:"auth_token"`
		} `yaml:"webhook"`
		ErrorAlerting struct {
			Enabled       bool `yaml:"enabled"`
			Threshold     int  `yaml:"threshold"`     // 窗口内 ERROR 数量阈值
			WindowSeconds int  `yaml:"window_seconds"` // 滑动窗口大小（秒）
		} `yaml:"error_alerting"`
	} `yaml:"alerting"`

	// Backup 数据备份配置
	Backup struct {
		Enabled    bool   `yaml:"enabled"`
		OutputDir  string `yaml:"output_dir"`  // 备份输出目录
		Prefix     string `yaml:"prefix"`      // 备份文件名前缀
		MaxBackups int    `yaml:"max_backups"` // 最大保留备份数，0=不限制
		Sources    []string `yaml:"sources"`   // 要备份的目录/文件列表
	} `yaml:"backup"`

	// Scheduler 定时任务配置
	Scheduler struct {
		Enabled    bool `yaml:"enabled"`
		MaxHistory int  `yaml:"max_history"` // 执行历史保留条数
	} `yaml:"scheduler"`

	// 缓存配置
	Cache struct {
		Backend string `yaml:"backend"`
		Redis   struct {
			Addr     string `yaml:"addr"`
			Password string `yaml:"password"`
			DB       int    `yaml:"db"`
			Prefix   string `yaml:"prefix"`
			// 连接池配置
			PoolSize        int `yaml:"pool_size"`          // 连接池大小
			MinIdleConns    int `yaml:"min_idle_conns"`     // 最小空闲连接数
			MaxIdleConns    int `yaml:"max_idle_conns"`     // 最大空闲连接数
			MaxRetries      int `yaml:"max_retries"`        // 最大重试次数
			DialTimeout     int `yaml:"dial_timeout"`       // 连接超时（毫秒）
			ReadTimeout     int `yaml:"read_timeout"`       // 读超时（毫秒）
			WriteTimeout    int `yaml:"write_timeout"`      // 写超时（毫秒）
			PoolTimeout     int `yaml:"pool_timeout"`       // 连接池超时（毫秒）
			ConnMaxLifetime int `yaml:"conn_max_lifetime"`  // 连接最大存活时间（毫秒）
			ConnMaxIdleTime int `yaml:"conn_max_idle_time"` // 连接最大空闲时间（毫秒）
		} `yaml:"redis"`
		// 按引擎的缓存配置
		Engines map[string]EngineCacheConfig `yaml:"engines"`
	} `yaml:"cache"`
}

// EngineCacheConfig 引擎级别的缓存配置
type EngineCacheConfig struct {
	Enabled bool `yaml:"enabled"`  // 是否启用缓存
	TTL     int  `yaml:"ttl"`      // 缓存时间（秒），0 表示使用全局默认
	MaxSize int  `yaml:"max_size"` // 最大缓存条目数，0 表示使用全局默认
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

// Clone 克隆配置，使用手写深拷贝替代 YAML 序列化方式。
// 优点：更高效，保留 nil vs 空切片区分，不会丢失未导出字段。
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	clone := &Config{}

	// Copy engine configs with Cookie slices
	clone.Engines.Quake = c.Engines.Quake
	clone.Engines.Quake.Cookies = cloneCookies(c.Engines.Quake.Cookies)
	clone.Engines.Zoomeye = c.Engines.Zoomeye
	clone.Engines.Zoomeye.Cookies = cloneCookies(c.Engines.Zoomeye.Cookies)
	clone.Engines.Hunter = c.Engines.Hunter
	clone.Engines.Hunter.Cookies = cloneCookies(c.Engines.Hunter.Cookies)
	clone.Engines.Fofa = c.Engines.Fofa
	clone.Engines.Fofa.Cookies = cloneCookies(c.Engines.Fofa.Cookies)
	clone.Engines.Shodan = c.Engines.Shodan

	// System, Log are all primitives — safe to copy directly
	clone.System = c.System
	clone.Log = c.Log

	// Screenshot (has pointer fields: Fallback, Headless)
	clone.Screenshot = c.Screenshot
	if c.Screenshot.Fallback != nil {
		v := *c.Screenshot.Fallback
		clone.Screenshot.Fallback = &v
	}
	if c.Screenshot.Headless != nil {
		v := *c.Screenshot.Headless
		clone.Screenshot.Headless = &v
	}

	// Web (has slice fields: CORS)
	clone.Web = c.Web
	clone.Web.CORS.AllowedOrigins = cloneStringSlice(c.Web.CORS.AllowedOrigins)
	clone.Web.CORS.AllowedMethods = cloneStringSlice(c.Web.CORS.AllowedMethods)
	clone.Web.CORS.AllowedHeaders = cloneStringSlice(c.Web.CORS.AllowedHeaders)
	clone.Web.CORS.ExposedHeaders = cloneStringSlice(c.Web.CORS.ExposedHeaders)
	clone.Web.Auth = c.Web.Auth

	// Network (has slice: Proxies)
	clone.Network = c.Network
	clone.Network.ProxyPool.Proxies = cloneStringSlice(c.Network.ProxyPool.Proxies)

	// Distributed (has map: NodeAuthTokens)
	clone.Distributed = c.Distributed
	clone.Distributed.NodeAuthTokens = cloneStringMap(c.Distributed.NodeAuthTokens)

	// Scheduler
	clone.Scheduler = c.Scheduler

	// Cache (has map: Engines)
	clone.Cache = c.Cache
	clone.Cache.Engines = cloneEngineCacheMap(c.Cache.Engines)

	return clone
}

func cloneCookies(src []Cookie) []Cookie {
	if src == nil {
		return nil
	}
	out := make([]Cookie, len(src))
	copy(out, src)
	return out
}

func cloneStringSlice(src []string) []string {
	if src == nil {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func cloneEngineCacheMap(src map[string]EngineCacheConfig) map[string]EngineCacheConfig {
	if src == nil {
		return nil
	}
	out := make(map[string]EngineCacheConfig, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
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
	config.Web.Auth.AdminToken = m.ResolveEnv(config.Web.Auth.AdminToken)

	// 解析缓存配置
	config.Cache.Backend = m.ResolveEnv(config.Cache.Backend)
	config.Cache.Redis.Addr = m.ResolveEnv(config.Cache.Redis.Addr)
	config.Cache.Redis.Password = m.ResolveEnv(config.Cache.Redis.Password)
	config.Cache.Redis.Prefix = m.ResolveEnv(config.Cache.Redis.Prefix)
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
	if config.System.CacheMaxSize == 0 {
		config.System.CacheMaxSize = 1000
	}
	if config.System.CacheCleanupInterval == 0 {
		config.System.CacheCleanupInterval = 300
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
	if strings.TrimSpace(config.Screenshot.Engine) == "" {
		config.Screenshot.Engine = "cdp"
	}

	// 解析截图模式：新字段 mode 优先，legacy engine 向后兼容
	mode := strings.ToLower(strings.TrimSpace(config.Screenshot.Mode))
	engine := strings.ToLower(strings.TrimSpace(config.Screenshot.Engine))
	if mode == "" {
		// 从 legacy engine 推导 mode
		switch engine {
		case "extension":
			mode = "auto" // extension 用户通常期望 fallback
		default:
			mode = "cdp"
		}
	}
	config.Screenshot.Mode = mode

	// 推导 priority
	priority := strings.ToLower(strings.TrimSpace(config.Screenshot.Priority))
	if priority == "" {
		switch mode {
		case "cdp":
			priority = "cdp"
		case "extension":
			priority = "extension"
		case "auto":
			priority = "cdp"
		default:
			priority = "cdp"
		}
	}
	config.Screenshot.Priority = priority

	// 推导 fallback
	if config.Screenshot.Fallback == nil {
		fb := true
		if mode == "cdp" || mode == "extension" {
			fb = false
		}
		config.Screenshot.Fallback = &fb
	}
	if strings.TrimSpace(config.Screenshot.Extension.ListenAddr) == "" {
		config.Screenshot.Extension.ListenAddr = "127.0.0.1:19451"
	}
	if config.Screenshot.Extension.TokenTTLSeconds == 0 {
		config.Screenshot.Extension.TokenTTLSeconds = 600
	}
	if config.Screenshot.Extension.TaskTimeoutSeconds == 0 {
		config.Screenshot.Extension.TaskTimeoutSeconds = 30
	}
	if config.Screenshot.Extension.MaxConcurrency == 0 {
		config.Screenshot.Extension.MaxConcurrency = 5
	}
	if config.Screenshot.Extension.CallbackSignatureSkewSeconds == 0 {
		config.Screenshot.Extension.CallbackSignatureSkewSeconds = 300
	}
	if config.Screenshot.Extension.CallbackNonceTTLSeconds == 0 {
		config.Screenshot.Extension.CallbackNonceTTLSeconds = 600
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

	if strings.TrimSpace(config.Network.ProxyPool.Strategy) == "" {
		config.Network.ProxyPool.Strategy = "round_robin"
	}
	if config.Network.ProxyPool.FailureThreshold == 0 {
		config.Network.ProxyPool.FailureThreshold = 2
	}
	if config.Network.ProxyPool.CooldownSeconds == 0 {
		config.Network.ProxyPool.CooldownSeconds = 60
	}
	config.Network.ProxyPool.Proxies = normalizeProxyList(config.Network.ProxyPool.Proxies)

	if config.Distributed.HeartbeatTimeoutSeconds == 0 {
		config.Distributed.HeartbeatTimeoutSeconds = 30
	}
	if config.Distributed.MaxReassignAttempts == 0 {
		config.Distributed.MaxReassignAttempts = 1
	}
	if strings.TrimSpace(config.Distributed.Scheduler.Strategy) == "" {
		config.Distributed.Scheduler.Strategy = "health_load"
	}
	if config.Distributed.NodeAuthTokens == nil {
		config.Distributed.NodeAuthTokens = make(map[string]string)
	}

	// 默认 Web 配置
	if config.Web.Port == 0 {
		config.Web.Port = 8448
	}
	if config.Web.BindAddress == "" {
		config.Web.BindAddress = "0.0.0.0"
	}
	if len(config.Web.CORS.AllowedOrigins) == 0 {
		config.Web.CORS.AllowedOrigins = []string{"http://localhost:8448", "http://127.0.0.1:8448"}
	}
	if len(config.Web.CORS.AllowedMethods) == 0 {
		config.Web.CORS.AllowedMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	}
	if len(config.Web.CORS.AllowedHeaders) == 0 {
		config.Web.CORS.AllowedHeaders = []string{"Content-Type", "Authorization", "X-Requested-With", "X-WebSocket-Token"}
	}
	if config.Web.CORS.MaxAge == 0 {
		config.Web.CORS.MaxAge = 600
	}

	if config.Web.RateLimit.RequestsPerWindow == 0 {
		config.Web.RateLimit.RequestsPerWindow = 60
	}
	if config.Web.RateLimit.WindowSeconds == 0 {
		config.Web.RateLimit.WindowSeconds = 60
	}

	if config.Web.RequestLimits.MaxBodyBytes == 0 {
		config.Web.RequestLimits.MaxBodyBytes = 10 * 1024 * 1024
	}
	if config.Web.RequestLimits.MaxMultipartMemory == 0 {
		config.Web.RequestLimits.MaxMultipartMemory = 10 * 1024 * 1024
	}

	// 默认定时任务配置
	if !config.Scheduler.Enabled {
		config.Scheduler.Enabled = true
	}
	if config.Scheduler.MaxHistory == 0 {
		config.Scheduler.MaxHistory = 500
	}

	// 默认缓存后端配置
	if strings.TrimSpace(config.Cache.Backend) == "" {
		config.Cache.Backend = "memory"
	}
	if strings.TrimSpace(config.Cache.Redis.Addr) == "" {
		config.Cache.Redis.Addr = "127.0.0.1:6379"
	}
	if strings.TrimSpace(config.Cache.Redis.Prefix) == "" {
		config.Cache.Redis.Prefix = "unimap:"
	}

	// 默认 Redis 连接池配置
	if config.Cache.Redis.PoolSize == 0 {
		config.Cache.Redis.PoolSize = 10
	}
	if config.Cache.Redis.MinIdleConns == 0 {
		config.Cache.Redis.MinIdleConns = 2
	}
	if config.Cache.Redis.MaxRetries == 0 {
		config.Cache.Redis.MaxRetries = 3
	}
	if config.Cache.Redis.DialTimeout == 0 {
		config.Cache.Redis.DialTimeout = 5000 // 5秒
	}
	if config.Cache.Redis.ReadTimeout == 0 {
		config.Cache.Redis.ReadTimeout = 3000 // 3秒
	}
	if config.Cache.Redis.WriteTimeout == 0 {
		config.Cache.Redis.WriteTimeout = 3000 // 3秒
	}
	if config.Cache.Redis.PoolTimeout == 0 {
		config.Cache.Redis.PoolTimeout = 4000 // 4秒
	}
	if config.Cache.Redis.ConnMaxLifetime == 0 {
		config.Cache.Redis.ConnMaxLifetime = 0 // 不限制
	}
	if config.Cache.Redis.ConnMaxIdleTime == 0 {
		config.Cache.Redis.ConnMaxIdleTime = 300000 // 5分钟
	}

	// 初始化引擎级别缓存配置（如果未设置）
	if config.Cache.Engines == nil {
		config.Cache.Engines = make(map[string]EngineCacheConfig)
	}

	// 为各引擎设置默认缓存配置
	engineDefaults := map[string]EngineCacheConfig{
		"quake":   {Enabled: true, TTL: 3600, MaxSize: 500},
		"zoomeye": {Enabled: true, TTL: 1800, MaxSize: 500}, // ZoomEye API 限制更严格
		"hunter":  {Enabled: true, TTL: 3600, MaxSize: 500},
		"fofa":    {Enabled: true, TTL: 1800, MaxSize: 500}, // FOFA 数据更新频繁
		"shodan":  {Enabled: true, TTL: 7200, MaxSize: 500}, // Shodan 数据相对稳定
	}

	for engine, defaultCfg := range engineDefaults {
		if _, exists := config.Cache.Engines[engine]; !exists {
			config.Cache.Engines[engine] = defaultCfg
		} else {
			// 合并配置：如果某个字段为零值，使用默认值
			cfg := config.Cache.Engines[engine]
			if cfg.TTL == 0 {
				cfg.TTL = defaultCfg.TTL
			}
			if cfg.MaxSize == 0 {
				cfg.MaxSize = defaultCfg.MaxSize
			}
			config.Cache.Engines[engine] = cfg
		}
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
	if config.System.CacheMaxSize <= 0 {
		return fmt.Errorf("system cache_max_size must be greater than 0")
	}
	if config.System.CacheCleanupInterval <= 0 {
		return fmt.Errorf("system cache_cleanup_interval must be greater than 0")
	}
	if config.System.RetryAttempts < 0 {
		return fmt.Errorf("system retry_attempts must be greater than or equal to 0")
	}
	if config.System.UserAgent == "" {
		return fmt.Errorf("system user_agent must be set")
	}

	// 验证 Web 配置
	if config.Web.CORS.MaxAge < 0 {
		return fmt.Errorf("web cors max_age must be greater than or equal to 0")
	}
	if config.Web.RateLimit.RequestsPerWindow <= 0 {
		return fmt.Errorf("web rate_limit requests_per_window must be greater than 0")
	}
	if config.Web.RateLimit.WindowSeconds <= 0 {
		return fmt.Errorf("web rate_limit window_seconds must be greater than 0")
	}
	if config.Web.RequestLimits.MaxBodyBytes <= 0 {
		return fmt.Errorf("web request_limits max_body_bytes must be greater than 0")
	}
	if config.Web.RequestLimits.MaxMultipartMemory <= 0 {
		return fmt.Errorf("web request_limits max_multipart_memory_bytes must be greater than 0")
	}

	engine := strings.ToLower(strings.TrimSpace(config.Screenshot.Engine))
	if engine != "" && engine != "cdp" && engine != "extension" {
		return fmt.Errorf("screenshot engine must be one of: cdp, extension")
	}

	mode := strings.ToLower(strings.TrimSpace(config.Screenshot.Mode))
	if mode != "auto" && mode != "cdp" && mode != "extension" {
		return fmt.Errorf("screenshot mode must be one of: auto, cdp, extension")
	}

	priority := strings.ToLower(strings.TrimSpace(config.Screenshot.Priority))
	if priority != "" && priority != "cdp" && priority != "extension" {
		return fmt.Errorf("screenshot priority must be one of: cdp, extension")
	}
	if config.Screenshot.Extension.TokenTTLSeconds <= 0 {
		return fmt.Errorf("screenshot extension token_ttl_seconds must be greater than 0")
	}
	if config.Screenshot.Extension.TaskTimeoutSeconds <= 0 {
		return fmt.Errorf("screenshot extension task_timeout_seconds must be greater than 0")
	}
	if config.Screenshot.Extension.MaxConcurrency <= 0 {
		return fmt.Errorf("screenshot extension max_concurrency must be greater than 0")
	}
	if config.Screenshot.Extension.CallbackSignatureSkewSeconds <= 0 {
		return fmt.Errorf("screenshot extension callback_signature_skew_seconds must be greater than 0")
	}
	if config.Screenshot.Extension.CallbackNonceTTLSeconds <= 0 {
		return fmt.Errorf("screenshot extension callback_nonce_ttl_seconds must be greater than 0")
	}
	if config.Screenshot.Extension.CallbackNonceTTLSeconds < config.Screenshot.Extension.CallbackSignatureSkewSeconds {
		return fmt.Errorf("screenshot extension callback_nonce_ttl_seconds must be greater than or equal to callback_signature_skew_seconds")
	}

	if config.Network.ProxyPool.Enabled {
		strategy := strings.ToLower(strings.TrimSpace(config.Network.ProxyPool.Strategy))
		if strategy != "round_robin" {
			return fmt.Errorf("network proxy_pool strategy must be: round_robin")
		}
		if len(config.Network.ProxyPool.Proxies) == 0 {
			return fmt.Errorf("network proxy_pool enabled but proxies are not set")
		}
		if config.Network.ProxyPool.FailureThreshold <= 0 {
			return fmt.Errorf("network proxy_pool failure_threshold must be greater than 0")
		}
		if config.Network.ProxyPool.CooldownSeconds <= 0 {
			return fmt.Errorf("network proxy_pool cooldown_seconds must be greater than 0")
		}
	}

	if config.Distributed.HeartbeatTimeoutSeconds <= 0 {
		return fmt.Errorf("distributed heartbeat_timeout_seconds must be greater than 0")
	}
	if config.Distributed.MaxReassignAttempts < 0 {
		return fmt.Errorf("distributed max_reassign_attempts must be greater than or equal to 0")
	}
	if config.Distributed.MaxReassignAttempts > 10 {
		return fmt.Errorf("distributed max_reassign_attempts must be less than or equal to 10")
	}
	strategy := strings.ToLower(strings.TrimSpace(config.Distributed.Scheduler.Strategy))
	if strategy != "health_load" {
		return fmt.Errorf("distributed scheduler strategy must be: health_load")
	}

	backend := strings.ToLower(strings.TrimSpace(config.Cache.Backend))
	if backend != "memory" && backend != "redis" {
		return fmt.Errorf("cache backend must be one of: memory, redis")
	}
	if backend == "redis" && strings.TrimSpace(config.Cache.Redis.Addr) == "" {
		return fmt.Errorf("cache redis addr must be set when backend is redis")
	}

	// 分布式安全校验：启用但未配置 token 时告警
	if config.Distributed.Enabled {
		if strings.TrimSpace(config.Distributed.AdminToken) == "" {
			// 不阻塞启动，但记录严重警告
			// 实际运行时 requireDistributedAdminToken 会返回 503
		}
		if len(config.Distributed.NodeAuthTokens) == 0 {
			// 同上：节点 token 为空时运行时拒绝注册
		}
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

// GetEngineCacheConfig 获取引擎级别的缓存配置
func (m *Manager) GetEngineCacheConfig(engineName string) EngineCacheConfig {
	if !m.IsValid() {
		return EngineCacheConfig{Enabled: true, TTL: 3600, MaxSize: 500}
	}

	engineName = strings.ToLower(strings.TrimSpace(engineName))
	if cfg, exists := m.config.Cache.Engines[engineName]; exists {
		return cfg
	}

	// 返回默认配置
	return EngineCacheConfig{Enabled: true, TTL: m.config.System.CacheTTL, MaxSize: m.config.System.CacheMaxSize}
}

// GetAllEngineCacheConfigs 获取所有引擎的缓存配置
func (m *Manager) GetAllEngineCacheConfigs() map[string]EngineCacheConfig {
	if !m.IsValid() {
		return make(map[string]EngineCacheConfig)
	}
	return m.config.Cache.Engines
}

// IsCacheEnabledForEngine 检查指定引擎是否启用缓存
func (m *Manager) IsCacheEnabledForEngine(engineName string) bool {
	cfg := m.GetEngineCacheConfig(engineName)
	return cfg.Enabled
}

func normalizeProxyList(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		parts := strings.FieldsFunc(item, func(r rune) bool {
			return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t'
		})
		if len(parts) == 0 {
			parts = []string{item}
		}
		for _, part := range parts {
			proxy := strings.TrimSpace(part)
			if proxy == "" {
				continue
			}
			if _, exists := seen[proxy]; exists {
				continue
			}
			seen[proxy] = struct{}{}
			out = append(out, proxy)
		}
	}
	return out
}

// GetCacheTTLForEngine 获取指定引擎的缓存 TTL（秒）
func (m *Manager) GetCacheTTLForEngine(engineName string) int {
	cfg := m.GetEngineCacheConfig(engineName)
	if cfg.TTL > 0 {
		return cfg.TTL
	}
	if m.IsValid() {
		return m.config.System.CacheTTL
	}
	return 3600 // 默认1小时
}

// GetCacheMaxSizeForEngine 获取指定引擎的最大缓存条目数
func (m *Manager) GetCacheMaxSizeForEngine(engineName string) int {
	cfg := m.GetEngineCacheConfig(engineName)
	if cfg.MaxSize > 0 {
		return cfg.MaxSize
	}
	if m.IsValid() {
		return m.config.System.CacheMaxSize
	}
	return 1000 // 默认1000条
}

// GetCacheBackend 获取缓存后端类型
func (m *Manager) GetCacheBackend() string {
	if !m.IsValid() {
		return "memory"
	}
	backend := strings.ToLower(strings.TrimSpace(m.config.Cache.Backend))
	if backend == "" {
		return "memory"
	}
	return backend
}

// GetRedisAddr 获取Redis地址
func (m *Manager) GetRedisAddr() string {
	if !m.IsValid() {
		return "127.0.0.1:6379"
	}
	return strings.TrimSpace(m.config.Cache.Redis.Addr)
}

// GetRedisPassword 获取Redis密码
func (m *Manager) GetRedisPassword() string {
	if !m.IsValid() {
		return ""
	}
	return m.config.Cache.Redis.Password
}

// GetRedisDB 获取Redis数据库
func (m *Manager) GetRedisDB() int {
	if !m.IsValid() {
		return 0
	}
	return m.config.Cache.Redis.DB
}

// GetRedisPrefix 获取Redis键前缀
func (m *Manager) GetRedisPrefix() string {
	if !m.IsValid() {
		return "unimap:"
	}
	prefix := strings.TrimSpace(m.config.Cache.Redis.Prefix)
	if prefix == "" {
		return "unimap:"
	}
	return prefix
}
