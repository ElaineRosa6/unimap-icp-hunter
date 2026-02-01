package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// Asset 统一资产模型
type Asset struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	IP       string `gorm:"type:varchar(45);not null;index:idx_ip_port" json:"ip"`
	Port     int    `gorm:"type:int;not null;index:idx_ip_port" json:"port"`
	Protocol string `gorm:"type:varchar(20);not null" json:"protocol"` // http/https/tcp/udp
	Host     string `gorm:"type:varchar(255);index" json:"host"`       // 域名/主机名
	URL      string `gorm:"type:varchar(512);not null;index" json:"url"`

	// Web信息
	Title       string  `gorm:"type:varchar(512)" json:"title"`
	BodySnippet string  `gorm:"type:text" json:"body_snippet"`
	Server      string  `gorm:"type:varchar(255)" json:"server"`
	Headers     JSONMap `gorm:"type:json" json:"headers"`
	StatusCode  int     `gorm:"type:int" json:"status_code"`

	// 位置与网络信息
	CountryCode string `gorm:"type:varchar(10)" json:"country_code"`
	Region      string `gorm:"type:varchar(50)" json:"region"` // 省份
	City        string `gorm:"type:varchar(50)" json:"city"`
	ASN         string `gorm:"type:varchar(20)" json:"asn"`
	Org         string `gorm:"type:varchar(255)" json:"org"`
	ISP         string `gorm:"type:varchar(100)" json:"isp"`

	// 来源与时间
	Sources     StringArray `gorm:"type:json" json:"sources"` // 引擎来源
	Extra       JSONMap     `gorm:"type:json" json:"extra"`   // 扩展信息
	FirstSeenAt time.Time   `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"first_seen_at"`
	LastSeenAt  time.Time   `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"last_seen_at"`

	// 关联的最新检测记录
	LatestCheckID uint `gorm:"type:bigint;index" json:"latest_check_id"`

	CreatedAt time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (Asset) TableName() string {
	return "assets"
}

// ICPCheck ICP检测记录
type ICPCheck struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	AssetID        uint      `gorm:"type:bigint;not null;index:idx_asset_check" json:"asset_id"`
	CheckTime      time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP;index:idx_asset_check" json:"check_time"`
	URL            string    `gorm:"type:varchar(512);not null" json:"url"`
	HTTPStatusCode int       `gorm:"type:int" json:"http_status_code"`
	Title          string    `gorm:"type:varchar(512)" json:"title"`
	ICPCode        string    `gorm:"type:varchar(100)" json:"icp_code"` // 检测到的备案号

	// 检测结果: 0=疑似未备案, 1=已备案, 2=不确定/需人工复核
	IsRegistered int `gorm:"type:tinyint;not null;default:0" json:"is_registered"`

	// 检测方式: regex/whitelist/manual
	MatchMethod string `gorm:"type:varchar(20)" json:"match_method"`

	HTMLHash       string      `gorm:"type:varchar(64);index" json:"html_hash"` // HTML内容哈希
	ScreenshotPath string      `gorm:"type:varchar(512)" json:"screenshot_path"`
	ErrorMessage   string      `gorm:"type:text" json:"error_message"`
	Tags           StringArray `gorm:"type:json" json:"tags"` // 标签: new, online_again, manual_review

	CreatedAt time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (ICPCheck) TableName() string {
	return "icp_checks"
}

// ScanPolicy 扫描策略
type ScanPolicy struct {
	ID          uint        `gorm:"primaryKey" json:"id"`
	Name        string      `gorm:"type:varchar(100);not null;unique" json:"name"`
	UQL         string      `gorm:"type:text;not null" json:"uql"`
	Engines     StringArray `gorm:"type:json" json:"engines"` // 引擎列表
	PageSize    int         `gorm:"type:int;default:100" json:"page_size"`
	MaxRecords  int         `gorm:"type:int;default:5000" json:"max_records"`
	Ports       IntArray    `gorm:"type:json" json:"ports"` // 目标端口
	Enabled     bool        `gorm:"type:tinyint;default:1" json:"enabled"`
	Description string      `gorm:"type:text" json:"description"`

	CreatedAt time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (ScanPolicy) TableName() string {
	return "scan_policies"
}

// ScanTask 扫描任务
type ScanTask struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	PolicyID          uint      `gorm:"type:bigint;not null;index" json:"policy_id"`
	Status            string    `gorm:"type:varchar(20);not null;index" json:"status"` // pending/running/completed/failed
	StartTime         time.Time `gorm:"type:timestamp" json:"start_time"`
	EndTime           time.Time `gorm:"type:timestamp" json:"end_time"`
	TotalCandidates   int       `gorm:"type:int;default:0" json:"total_candidates"`
	TotalProbed       int       `gorm:"type:int;default:0" json:"total_probed"`
	TotalUnregistered int       `gorm:"type:int;default:0" json:"total_unregistered"`
	StatsSummary      JSONMap   `gorm:"type:json" json:"stats_summary"`
	ErrorMessage      string    `gorm:"type:text" json:"error_message"`

	CreatedAt time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (ScanTask) TableName() string {
	return "scan_tasks"
}

// Whitelist 白名单
type Whitelist struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Type      string    `gorm:"type:varchar(20);not null;index" json:"type"` // ip/domain/asn/org
	Value     string    `gorm:"type:varchar(512);not null" json:"value"`
	Reason    string    `gorm:"type:text" json:"reason"`
	CreatedBy string    `gorm:"type:varchar(100)" json:"creator"`
	CreatedAt time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (Whitelist) TableName() string {
	return "whitelist"
}

// JSONMap 用于处理JSON类型的字段
type JSONMap map[string]interface{}

func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	data, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(data, j)
}

// StringArray 用于处理JSON数组类型的字段
type StringArray []string

func (s StringArray) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

func (s *StringArray) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	data, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(data, s)
}

// IntArray 用于处理JSON数组类型的字段
type IntArray []int

func (i IntArray) Value() (driver.Value, error) {
	if i == nil {
		return nil, nil
	}
	return json.Marshal(i)
}

func (i *IntArray) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	data, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(data, i)
}
