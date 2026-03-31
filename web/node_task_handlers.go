package web

import (
	"net/http"
	"strings"

	"github.com/unimap-icp-hunter/project/internal/distributed"
)

func (s *Server) handleNodeTaskEnqueue(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !s.requireDistributedEnabled(w) {
		return
	}
	if !s.requireDistributedAdminToken(w, r) {
		return
	}
	if s.nodeTaskQueue == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "node_task_queue_unavailable", "node task queue not initialized", nil)
		return
	}

	var req distributed.TaskEnvelope
	if !decodeJSONBody(w, r, &req) {
		return
	}

	rec, err := s.nodeTaskQueue.Enqueue(req)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "node_task_enqueue_failed", "node task enqueue failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "task": rec})
}

func (s *Server) handleNodeTaskClaim(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !s.requireDistributedEnabled(w) {
		return
	}
	if s.nodeTaskQueue == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "node_task_queue_unavailable", "node task queue not initialized", nil)
		return
	}

	var req struct {
		NodeID string   `json:"node_id"`
		Caps   []string `json:"caps"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !s.requireNodeToken(w, r, req.NodeID) {
		return
	}

	rec, err := s.nodeTaskQueue.Claim(req.NodeID, req.Caps)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "node_task_claim_failed", "node task claim failed", err.Error())
		return
	}
	if rec == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "task": nil, "message": "no task available"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "task": rec})
}

func (s *Server) handleNodeTaskResult(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !s.requireDistributedEnabled(w) {
		return
	}
	if s.nodeTaskQueue == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "node_task_queue_unavailable", "node task queue not initialized", nil)
		return
	}

	var req distributed.TaskResult
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !s.requireNodeToken(w, r, req.NodeID) {
		return
	}
	if strings.TrimSpace(req.Status) == "" {
		writeAPIError(w, http.StatusBadRequest, "node_task_result_failed", "node task result failed", "status is required")
		return
	}

	rec, err := s.nodeTaskQueue.SubmitResult(req)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "node_task_result_failed", "node task result failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "task": rec})
}

func (s *Server) handleNodeTaskStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if !s.requireDistributedEnabled(w) {
		return
	}
	if !s.requireDistributedAdminToken(w, r) {
		return
	}
	if s.nodeTaskQueue == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "node_task_queue_unavailable", "node task queue not initialized", nil)
		return
	}

	tasks := s.nodeTaskQueue.Snapshot()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"summary": map[string]int{"total": len(tasks)},
		"tasks":   tasks,
	})
}
