package distributed

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Registry stores node liveness and runtime metadata in memory.
type Registry struct {
	nodes            map[string]*NodeRecord
	heartbeatTimeout time.Duration
	mu               sync.RWMutex
}

func NewRegistry(heartbeatTimeout time.Duration) *Registry {
	if heartbeatTimeout <= 0 {
		heartbeatTimeout = 30 * time.Second
	}
	return &Registry{
		nodes:            make(map[string]*NodeRecord),
		heartbeatTimeout: heartbeatTimeout,
	}
}

func (r *Registry) Register(req NodeRegistration) (NodeRecord, error) {
	nodeID := strings.TrimSpace(req.NodeID)
	if nodeID == "" {
		return NodeRecord{}, fmt.Errorf("node_id is required")
	}
	if req.MaxConcurrency < 0 {
		return NodeRecord{}, fmt.Errorf("max_concurrency must be greater than or equal to 0")
	}

	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	record, exists := r.nodes[nodeID]
	if !exists {
		record = &NodeRecord{NodeID: nodeID, RegisteredAt: now}
		r.nodes[nodeID] = record
	}

	record.Hostname = strings.TrimSpace(req.Hostname)
	record.Region = strings.TrimSpace(req.Region)
	record.Labels = cloneLabels(req.Labels)
	record.Capabilities = cloneStringSlice(req.Capabilities)
	record.Version = strings.TrimSpace(req.Version)
	record.EgressIP = strings.TrimSpace(req.EgressIP)
	if req.MaxConcurrency > 0 {
		record.MaxConcurrency = req.MaxConcurrency
	}
	if record.MaxConcurrency <= 0 {
		record.MaxConcurrency = 1
	}
	record.LastHeartbeatAt = now
	record.Online = true

	return *record, nil
}

func (r *Registry) Heartbeat(hb NodeHeartbeat) (NodeRecord, error) {
	nodeID := strings.TrimSpace(hb.NodeID)
	if nodeID == "" {
		return NodeRecord{}, fmt.Errorf("node_id is required")
	}

	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	record, exists := r.nodes[nodeID]
	if !exists {
		record = &NodeRecord{NodeID: nodeID, RegisteredAt: now, MaxConcurrency: 1}
		r.nodes[nodeID] = record
	}

	if hb.CurrentLoad >= 0 {
		record.CurrentLoad = hb.CurrentLoad
	}
	if hb.MaxConcurrency > 0 {
		record.MaxConcurrency = hb.MaxConcurrency
	}
	if record.MaxConcurrency <= 0 {
		record.MaxConcurrency = 1
	}
	if hb.AvgLatencyMS >= 0 {
		record.AvgLatencyMS = hb.AvgLatencyMS
	}
	if hb.SuccessRate5m >= 0 {
		record.SuccessRate5m = hb.SuccessRate5m
	}
	if v := strings.TrimSpace(hb.Version); v != "" {
		record.Version = v
	}
	if egress := strings.TrimSpace(hb.EgressIP); egress != "" {
		record.EgressIP = egress
	}

	record.LastHeartbeatAt = now
	record.Online = true
	return *record, nil
}

func (r *Registry) Snapshot() NodeStatusSnapshot {
	now := time.Now()
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodes := make([]NodeRecord, 0, len(r.nodes))
	online := 0
	offline := 0
	for _, record := range r.nodes {
		item := *record
		if now.Sub(item.LastHeartbeatAt) > r.heartbeatTimeout {
			item.Online = false
		}
		if item.Online {
			online++
		} else {
			offline++
		}
		nodes = append(nodes, item)
	}

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].NodeID < nodes[j].NodeID
	})

	return NodeStatusSnapshot{
		Total:   len(nodes),
		Online:  online,
		Offline: offline,
		Nodes:   nodes,
	}
}

func cloneLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = strings.TrimSpace(v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, item := range in {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
