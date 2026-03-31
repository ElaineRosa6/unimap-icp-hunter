package web

import (
	"net/http"
	"sort"
	"strings"

	"github.com/unimap-icp-hunter/project/internal/distributed"
)

func (s *Server) isDistributedEnabled() bool {
	if s == nil {
		return false
	}
	if s.config == nil {
		return true
	}
	return s.config.Distributed.Enabled
}

func (s *Server) requireDistributedEnabled(w http.ResponseWriter) bool {
	if !s.isDistributedEnabled() {
		writeAPIError(w, http.StatusServiceUnavailable, "distributed_disabled", "distributed mode is disabled", nil)
		return false
	}
	return true
}

func (s *Server) handleNodeRegister(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !s.requireDistributedEnabled(w) {
		return
	}
	if s.nodeRegistry == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "node_registry_unavailable", "node registry not initialized", nil)
		return
	}

	var req distributed.NodeRegistration
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !s.requireNodeToken(w, r, req.NodeID) {
		return
	}

	record, err := s.nodeRegistry.Register(req)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "node_register_failed", "node register failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"node":    record,
	})
}

func (s *Server) handleNodeHeartbeat(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !s.requireDistributedEnabled(w) {
		return
	}
	if s.nodeRegistry == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "node_registry_unavailable", "node registry not initialized", nil)
		return
	}

	var req distributed.NodeHeartbeat
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !s.requireNodeToken(w, r, req.NodeID) {
		return
	}

	record, err := s.nodeRegistry.Heartbeat(req)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "node_heartbeat_failed", "node heartbeat failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"node":    record,
	})
}

func (s *Server) handleNodeStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if !s.requireDistributedEnabled(w) {
		return
	}
	if !s.requireDistributedAdminToken(w, r) {
		return
	}
	if s.nodeRegistry == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "node_registry_unavailable", "node registry not initialized", nil)
		return
	}

	snapshot := s.nodeRegistry.Snapshot()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"summary": map[string]int{
			"total":   snapshot.Total,
			"online":  snapshot.Online,
			"offline": snapshot.Offline,
		},
		"nodes": snapshot.Nodes,
	})
}

func (s *Server) handleNodeNetworkProfile(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if !s.requireDistributedEnabled(w) {
		return
	}
	if !s.requireDistributedAdminToken(w, r) {
		return
	}
	if s.nodeRegistry == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "node_registry_unavailable", "node registry not initialized", nil)
		return
	}

	snapshot := s.nodeRegistry.Snapshot()
	profiles := make([]map[string]interface{}, 0, len(snapshot.Nodes))
	egressCount := make(map[string]int)
	for _, node := range snapshot.Nodes {
		egress := strings.TrimSpace(node.EgressIP)
		if egress != "" {
			egressCount[egress]++
		}
		profiles = append(profiles, map[string]interface{}{
			"node_id":           node.NodeID,
			"online":            node.Online,
			"egress_ip":         egress,
			"region":            node.Region,
			"avg_latency_ms":    node.AvgLatencyMS,
			"success_rate_5m":   node.SuccessRate5m,
			"current_load":      node.CurrentLoad,
			"max_concurrency":   node.MaxConcurrency,
			"last_heartbeat_at": node.LastHeartbeatAt,
		})
	}

	egressSummary := make([]map[string]interface{}, 0, len(egressCount))
	for egress, count := range egressCount {
		egressSummary = append(egressSummary, map[string]interface{}{"egress_ip": egress, "nodes": count})
	}
	sort.Slice(egressSummary, func(i, j int) bool {
		a := strings.TrimSpace(egressSummary[i]["egress_ip"].(string))
		b := strings.TrimSpace(egressSummary[j]["egress_ip"].(string))
		return a < b
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"summary": map[string]int{
			"total":   snapshot.Total,
			"online":  snapshot.Online,
			"offline": snapshot.Offline,
		},
		"egress_summary": egressSummary,
		"profiles":       profiles,
	})
}
