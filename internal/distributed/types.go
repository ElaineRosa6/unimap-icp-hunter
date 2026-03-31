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
}

// NodeStatusSnapshot summarizes current node states.
type NodeStatusSnapshot struct {
	Total   int          `json:"total"`
	Online  int          `json:"online"`
	Offline int          `json:"offline"`
	Nodes   []NodeRecord `json:"nodes"`
}
