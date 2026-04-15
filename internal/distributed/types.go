package distributed

import "time"

// NodeRegistration represents initial or refresh registration payload from a node.
type NodeRegistration struct {
	NodeID         string            `json:"node_id"`
	Hostname       string            `json:"hostname,omitempty"`
	Region         string            `json:"region,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
	Capabilities   []string          `json:"capabilities,omitempty"`
	MaxConcurrency int               `json:"max_concurrency,omitempty"`
	Version        string            `json:"version,omitempty"`
	EgressIP       string            `json:"egress_ip,omitempty"`
}

// NodeHeartbeat updates node liveness and runtime load.
type NodeHeartbeat struct {
	NodeID         string  `json:"node_id"`
	CurrentLoad    int     `json:"current_load,omitempty"`
	MaxConcurrency int     `json:"max_concurrency,omitempty"`
	AvgLatencyMS   float64 `json:"avg_latency_ms,omitempty"`
	SuccessRate5m  float64 `json:"success_rate_5m,omitempty"`
	Version        string  `json:"version,omitempty"`
	EgressIP       string  `json:"egress_ip,omitempty"`

	// 健康检查指标
	CPUUsage       float64         `json:"cpu_usage,omitempty"`          // CPU使用率 (0-100)
	MemoryUsage    float64         `json:"memory_usage,omitempty"`       // 内存使用率 (0-100)
	DiskUsage      float64         `json:"disk_usage,omitempty"`         // 磁盘使用率 (0-100)
	NetworkLatency float64         `json:"network_latency_ms,omitempty"` // 网络延迟(ms)
	ErrorRate      float64         `json:"error_rate,omitempty"`         // 错误率 (0-1)
	ActiveTasks    int             `json:"active_tasks,omitempty"`       // 活跃任务数
	HealthChecks   map[string]bool `json:"health_checks,omitempty"`      // 健康检查项状态
}

// NodeRecord is the controller-side in-memory node state.
type NodeRecord struct {
	NodeID          string            `json:"node_id"`
	Hostname        string            `json:"hostname,omitempty"`
	Region          string            `json:"region,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Capabilities    []string          `json:"capabilities,omitempty"`
	Version         string            `json:"version,omitempty"`
	EgressIP        string            `json:"egress_ip,omitempty"`
	CurrentLoad     int               `json:"current_load"`
	MaxConcurrency  int               `json:"max_concurrency"`
	AvgLatencyMS    float64           `json:"avg_latency_ms,omitempty"`
	SuccessRate5m   float64           `json:"success_rate_5m,omitempty"`
	LastHeartbeatAt time.Time         `json:"last_heartbeat_at"`
	RegisteredAt    time.Time         `json:"registered_at"`
	Online          bool              `json:"online"`

	// 健康状态相关字段
	CPUUsage       float64         `json:"cpu_usage,omitempty"`          // CPU使用率 (0-100)
	MemoryUsage    float64         `json:"memory_usage,omitempty"`       // 内存使用率 (0-100)
	DiskUsage      float64         `json:"disk_usage,omitempty"`         // 磁盘使用率 (0-100)
	NetworkLatency float64         `json:"network_latency_ms,omitempty"` // 网络延迟(ms)
	ErrorRate      float64         `json:"error_rate,omitempty"`         // 错误率 (0-1)
	ActiveTasks    int             `json:"active_tasks,omitempty"`       // 活跃任务数
	HealthChecks   map[string]bool `json:"health_checks,omitempty"`      // 健康检查项状态
	HealthScore    float64         `json:"health_score,omitempty"`       // 健康评分 (0-100)
	HealthStatus   string          `json:"health_status,omitempty"`      // 健康状态：healthy, warning, critical, offline

	// 故障转移相关字段
	FailoverCount     int       `json:"failover_count,omitempty"`      // 故障转移次数
	LastFailoverAt    time.Time `json:"last_failover_at,omitempty"`    // 上次故障转移时间
	TaskRecoveryCount int       `json:"task_recovery_count,omitempty"` // 任务恢复次数
}

// FailoverStrategy 故障转移策略类型
type FailoverStrategy string

const (
	// FailoverStrategyHealthBased 基于健康状态的故障转移
	FailoverStrategyHealthBased FailoverStrategy = "health_based"
	// FailoverStrategyLoadBalanced 负载均衡的故障转移
	FailoverStrategyLoadBalanced FailoverStrategy = "load_balanced"
	// FailoverStrategyPriorityBased 基于优先级的故障转移
	FailoverStrategyPriorityBased FailoverStrategy = "priority_based"
)

// FailoverManager 故障转移管理器接口
type FailoverManager interface {
	// HandleNodeFailure 处理节点故障
	HandleNodeFailure(nodeID string) error
	// GetHealthyNodes 获取健康节点列表
	GetHealthyNodes() []*NodeRecord
	// SetStrategy 设置故障转移策略
	SetStrategy(strategy FailoverStrategy)
}

// NodeStatusSnapshot summarizes current node states.
type NodeStatusSnapshot struct {
	Total   int          `json:"total"`
	Online  int          `json:"online"`
	Offline int          `json:"offline"`
	Nodes   []NodeRecord `json:"nodes"`
}
