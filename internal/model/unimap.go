package model

// UQLNode UQL语法树节点
type UQLNode struct {
	Type     string     `json:"type"`     // operator, field, value
	Value    string     `json:"value"`    // 字段名或操作符
	Children []*UQLNode `json:"children"` // 子节点
}

// UQLAST UQL抽象语法树
type UQLAST struct {
	Root *UQLNode `json:"root"`
}

// EngineQuery 引擎查询结构
type EngineQuery struct {
	EngineName string `json:"engine_name"`
	Query      string `json:"query"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
}

// EngineResult 引擎返回的原始结果
type EngineResult struct {
	EngineName     string           `json:"engine_name"`
	RawData        []interface{}    `json:"raw_data"`
	Total          int              `json:"total"`
	Page           int              `json:"page"`
	HasMore        bool             `json:"has_more"`
	Error          string           `json:"error,omitempty"`
	Cached         bool             `json:"cached,omitempty"`         // 标记是否来自缓存
	NormalizedData []UnifiedAsset   `json:"normalized_data,omitempty"` // 缓存命中时的标准化数据
}

// UnifiedAsset 统一资产结构 (用于引擎适配器返回)
type UnifiedAsset struct {
	IP          string                 `json:"ip"`
	Port        int                    `json:"port"`
	Protocol    string                 `json:"protocol"`
	Host        string                 `json:"host"`
	URL         string                 `json:"url"`
	Title       string                 `json:"title"`
	BodySnippet string                 `json:"body_snippet"`
	Server      string                 `json:"server"`
	Headers     map[string]string      `json:"headers"`
	StatusCode  int                    `json:"status_code"`
	CountryCode string                 `json:"country_code"`
	Region      string                 `json:"region"`
	City        string                 `json:"city"`
	ASN         string                 `json:"asn"`
	Org         string                 `json:"org"`
	ISP         string                 `json:"isp"`
	Source      string                 `json:"source"`
	Extra       map[string]interface{} `json:"extra"`
}

// QuotaInfo 引擎配额信息
type QuotaInfo struct {
	Remaining int    `json:"remaining"` // 剩余配额
	Total     int    `json:"total"`     // 总配额
	Used      int    `json:"used"`      // 已使用配额
	Unit      string `json:"unit"`      // 配额单位（如 "times", "assets"）
	Expiry    string `json:"expiry"`    // 过期时间
}

// EngineAdapter 引擎适配器接口
type EngineAdapter interface {
	Name() string
	Translate(ast *UQLAST) (string, error)
	Search(query string, page, pageSize int) (*EngineResult, error)
	Normalize(raw *EngineResult) ([]UnifiedAsset, error)
	GetQuota() (*QuotaInfo, error) // 获取配额信息
}

// FieldMapping 引擎字段映射
type FieldMapping struct {
	Unified string   `yaml:"unified"`
	Engine  string   `yaml:"engine"`
	Fields  []string `yaml:"fields"`
}

// EngineConfig 引擎配置
type EngineConfig struct {
	Enabled    bool   `yaml:"enabled"`
	BaseURL    string `yaml:"base_url"`
	APIKey     string `yaml:"api_key"`
	Email      string `yaml:"email"`
	QPS        int    `yaml:"qps"`
	Timeout    int    `yaml:"timeout"`
	MaxRetries int    `yaml:"max_retries"`
}

// MergeResult 归并结果
type MergeResult struct {
	Assets     map[string]*UnifiedAsset `json:"assets"` // key: ip:port
	Total      int                      `json:"total"`
	Duplicates int                      `json:"duplicates"`
}
