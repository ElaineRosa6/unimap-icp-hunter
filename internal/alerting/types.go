package alerting

import (
	"time"
)

// AlertLevel 告警级别
type AlertLevel string

const (
	// AlertLevelInfo 信息级别
	AlertLevelInfo AlertLevel = "info"
	// AlertLevelWarning 警告级别
	AlertLevelWarning AlertLevel = "warning"
	// AlertLevelError 错误级别
	AlertLevelError AlertLevel = "error"
	// AlertLevelCritical 严重级别
	AlertLevelCritical AlertLevel = "critical"
)

// AlertType 告警类型
type AlertType string

const (
	// AlertTypeTamper 篡改检测告警
	AlertTypeTamper AlertType = "tamper"
	// AlertTypeReachability 可达性告警
	AlertTypeReachability AlertType = "reachability"
	// AlertTypeSecurity 安全告警
	AlertTypeSecurity AlertType = "security"
	// AlertTypeSystem 系统告警
	AlertTypeSystem AlertType = "system"
	// AlertTypePerformance 性能告警
	AlertTypePerformance AlertType = "performance"
)

// Alert 告警信息
type Alert struct {
	ID        string      `json:"id"`
	Level     AlertLevel  `json:"level"`
	Type      AlertType   `json:"type"`
	Title     string      `json:"title"`
	Message   string      `json:"message"`
	Details   interface{} `json:"details,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	Source    string      `json:"source"`
	URL       string      `json:"url,omitempty"`
}

// AlertChannel 告警通知渠道接口
type AlertChannel interface {
	// Name 返回渠道名称
	Name() string
	// Send 发送告警
	Send(alert Alert) error
	// IsEnabled 检查渠道是否启用
	IsEnabled() bool
	// Close 关闭渠道
	Close() error
}

// ChannelConfig 渠道配置
type ChannelConfig struct {
	Type     string                 `json:"type"`
	Enabled  bool                   `json:"enabled"`
	Settings map[string]interface{} `json:"settings"`
}

// AlertThreshold 告警阈值配置
type AlertThreshold struct {
	Type       string  `json:"type"`        // 阈值类型：tamper_segments, tamper_frequency, response_time, error_rate
	Value      float64 `json:"value"`       // 阈值值
	WindowSize int     `json:"window_size"` // 时间窗口大小（秒）
	Enabled    bool    `json:"enabled"`     // 是否启用
}

// AlertConfig 告警配置
type AlertConfig struct {
	Thresholds     []AlertThreshold `json:"thresholds"`     // 告警阈值配置
	Silence        SilenceConfig    `json:"silence"`        // 静默配置
	Acknowledgment bool             `json:"acknowledgment"` // 是否启用确认机制
}

// SilenceConfig 静默配置
type SilenceConfig struct {
	Enabled     bool `json:"enabled"`      // 是否启用静默
	Duration    int  `json:"duration"`     // 静默时长（秒）
	MinInterval int  `json:"min_interval"` // 最小告警间隔（秒）
	MaxAlerts   int  `json:"max_alerts"`   // 最大告警数量
	ByType      bool `json:"by_type"`      // 是否按类型静默
	BySource    bool `json:"by_source"`    // 是否按来源静默
	ByURL       bool `json:"by_url"`       // 是否按URL静默
}

// AlertStatus 告警状态
type AlertStatus string

const (
	// AlertStatusNew 新告警
	AlertStatusNew AlertStatus = "new"
	// AlertStatusAcknowledged 已确认
	AlertStatusAcknowledged AlertStatus = "acknowledged"
	// AlertStatusSilenced 已静默
	AlertStatusSilenced AlertStatus = "silenced"
	// AlertStatusResolved 已解决
	AlertStatusResolved AlertStatus = "resolved"
)

// AcknowledgmentInfo 确认信息
type AcknowledgmentInfo struct {
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name"`
	Timestamp time.Time `json:"timestamp"`
	Comment   string    `json:"comment"`
}

// AlertRecord 告警记录（用于状态跟踪）
type AlertRecord struct {
	Alert          Alert               `json:"alert"`
	Status         AlertStatus         `json:"status"`
	Acknowledgment *AcknowledgmentInfo `json:"acknowledgment,omitempty"`
	SilenceUntil   *time.Time          `json:"silence_until,omitempty"`
	LastModified   time.Time           `json:"last_modified"`
}
