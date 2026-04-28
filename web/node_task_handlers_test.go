package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/unimap-icp-hunter/project/internal/config"
	"github.com/unimap-icp-hunter/project/internal/distributed"
)

func TestNodeTaskFlow(t *testing.T) {
	cfg := &config.Config{}
	cfg.Distributed.Enabled = true
	cfg.Distributed.AdminToken = "test-admin-token"
	cfg.Distributed.NodeAuthTokens = map[string]string{"node-a": "node-token-a"}
	s := &Server{
		distributed: &DistributedState{
			NodeRegistry:  distributed.NewRegistry(60 * time.Second),
			NodeTaskQueue: distributed.NewTaskQueue(),
		},
		config: cfg,
	}

	enqueueBody := map[string]interface{}{
		"task_id":       "task-1",
		"task_type":     "port_scan",
		"priority":      10,
		"required_caps": []string{"port_scan"},
		"payload":       map[string]interface{}{"url": "https://example.com"},
	}
	enqueueBytes, _ := json.Marshal(enqueueBody)
	enqueueReq := httptest.NewRequest(http.MethodPost, "/api/nodes/task/enqueue", bytes.NewReader(enqueueBytes))
	enqueueReq.Header.Set("Authorization", "Bearer test-admin-token")
	enqueueW := httptest.NewRecorder()
	s.handleNodeTaskEnqueue(enqueueW, enqueueReq)
	if enqueueW.Code != http.StatusOK {
		t.Fatalf("enqueue expected 200, got %d, body=%s", enqueueW.Code, enqueueW.Body.String())
	}

	claimBody := map[string]interface{}{"node_id": "node-a", "caps": []string{"port_scan"}}
	claimBytes, _ := json.Marshal(claimBody)
	claimReq := httptest.NewRequest(http.MethodPost, "/api/nodes/task/claim", bytes.NewReader(claimBytes))
	claimReq.Header.Set("Authorization", "Bearer node-token-a")
	claimW := httptest.NewRecorder()
	s.handleNodeTaskClaim(claimW, claimReq)
	if claimW.Code != http.StatusOK {
		t.Fatalf("claim expected 200, got %d, body=%s", claimW.Code, claimW.Body.String())
	}

	resultBody := map[string]interface{}{"task_id": "task-1", "node_id": "node-a", "status": "completed", "duration_ms": 32}
	resultBytes, _ := json.Marshal(resultBody)
	resultReq := httptest.NewRequest(http.MethodPost, "/api/nodes/task/result", bytes.NewReader(resultBytes))
	resultReq.Header.Set("Authorization", "Bearer node-token-a")
	resultW := httptest.NewRecorder()
	s.handleNodeTaskResult(resultW, resultReq)
	if resultW.Code != http.StatusOK {
		t.Fatalf("result expected 200, got %d, body=%s", resultW.Code, resultW.Body.String())
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/nodes/task/status", nil)
	statusReq.Header.Set("Authorization", "Bearer test-admin-token")
	statusW := httptest.NewRecorder()
	s.handleNodeTaskStatus(statusW, statusReq)
	if statusW.Code != http.StatusOK {
		t.Fatalf("status expected 200, got %d, body=%s", statusW.Code, statusW.Body.String())
	}

	var resp struct {
		Success bool `json:"success"`
		Summary struct {
			Total int `json:"total"`
		} `json:"summary"`
		Tasks []struct {
			TaskID string `json:"task_id"`
			Status string `json:"status"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(statusW.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if !resp.Success || resp.Summary.Total != 1 {
		t.Fatalf("unexpected status response: %+v", resp)
	}
	if len(resp.Tasks) != 1 || resp.Tasks[0].TaskID != "task-1" || resp.Tasks[0].Status != "completed" {
		t.Fatalf("unexpected task snapshot: %+v", resp.Tasks)
	}
}

func TestNodeTask_DistributedDisabled(t *testing.T) {
	s := &Server{distributed: &DistributedState{NodeTaskQueue: distributed.NewTaskQueue()}, config: &config.Config{}}
	body := map[string]interface{}{"task_id": "task-1", "task_type": "port_scan"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/nodes/task/enqueue", bytes.NewReader(b))
	w := httptest.NewRecorder()
	s.handleNodeTaskEnqueue(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when distributed disabled, got %d", w.Code)
	}
}

func TestNodeTaskClaim_NodeAuthToken(t *testing.T) {
	cfg := &config.Config{}
	cfg.Distributed.Enabled = true
	cfg.Distributed.NodeAuthTokens = map[string]string{"node-a": "token-a"}
	s := &Server{distributed: &DistributedState{NodeTaskQueue: distributed.NewTaskQueue()}, config: cfg}

	_, err := s.distributed.NodeTaskQueue.Enqueue(distributed.TaskEnvelope{TaskID: "task-auth-1", TaskType: "port_scan"})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	body := map[string]interface{}{"node_id": "node-a", "caps": []string{"port_scan"}}
	b, _ := json.Marshal(body)

	unauthReq := httptest.NewRequest(http.MethodPost, "/api/nodes/task/claim", bytes.NewReader(b))
	unauthW := httptest.NewRecorder()
	s.handleNodeTaskClaim(unauthW, unauthReq)
	if unauthW.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d, body=%s", unauthW.Code, unauthW.Body.String())
	}

	authReq := httptest.NewRequest(http.MethodPost, "/api/nodes/task/claim", bytes.NewReader(b))
	authReq.Header.Set("Authorization", "Bearer token-a")
	authW := httptest.NewRecorder()
	s.handleNodeTaskClaim(authW, authReq)
	if authW.Code != http.StatusOK {
		t.Fatalf("expected 200 with token, got %d, body=%s", authW.Code, authW.Body.String())
	}
}

func TestNodeTaskEnqueue_AdminToken(t *testing.T) {
	cfg := &config.Config{}
	cfg.Distributed.Enabled = true
	cfg.Distributed.AdminToken = "admin-token"
	s := &Server{distributed: &DistributedState{NodeTaskQueue: distributed.NewTaskQueue()}, config: cfg}

	body := map[string]interface{}{"task_id": "task-admin-1", "task_type": "port_scan"}
	b, _ := json.Marshal(body)

	unauthReq := httptest.NewRequest(http.MethodPost, "/api/nodes/task/enqueue", bytes.NewReader(b))
	unauthW := httptest.NewRecorder()
	s.handleNodeTaskEnqueue(unauthW, unauthReq)
	if unauthW.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without admin token, got %d, body=%s", unauthW.Code, unauthW.Body.String())
	}

	authReq := httptest.NewRequest(http.MethodPost, "/api/nodes/task/enqueue", bytes.NewReader(b))
	authReq.Header.Set("Authorization", "Bearer admin-token")
	authW := httptest.NewRecorder()
	s.handleNodeTaskEnqueue(authW, authReq)
	if authW.Code != http.StatusOK {
		t.Fatalf("expected 200 with admin token, got %d, body=%s", authW.Code, authW.Body.String())
	}
}

// ============================================================
// handleNodeTaskGet tests
// ============================================================

func TestHandleNodeTaskGet_DistributedDisabled(t *testing.T) {
	s := &Server{distributed: &DistributedState{NodeTaskQueue: distributed.NewTaskQueue()}, config: &config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/api/nodes/task/task-1", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	w := httptest.NewRecorder()
	s.handleNodeTaskGet(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when distributed disabled, got %d", w.Code)
	}
}

func TestHandleNodeTaskGet_NotFound(t *testing.T) {
	cfg := &config.Config{}
	cfg.Distributed.Enabled = true
	cfg.Distributed.AdminToken = "admin-token"
	s := &Server{distributed: &DistributedState{NodeTaskQueue: distributed.NewTaskQueue()}, config: cfg}

	req := httptest.NewRequest(http.MethodGet, "/api/nodes/task/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	w := httptest.NewRecorder()
	s.handleNodeTaskGet(w, req)

	if w.Code != http.StatusNotFound && w.Code != http.StatusBadRequest {
		t.Fatalf("expected 404 or 400 for nonexistent task, got %d", w.Code)
	}
}

// ============================================================
// handleNodeTaskDelete tests
// ============================================================

func TestHandleNodeTaskDelete_DistributedDisabled(t *testing.T) {
	s := &Server{distributed: &DistributedState{NodeTaskQueue: distributed.NewTaskQueue()}, config: &config.Config{}}
	req := httptest.NewRequest(http.MethodDelete, "/api/nodes/task/task-1", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	w := httptest.NewRecorder()
	s.handleNodeTaskDelete(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when distributed disabled, got %d", w.Code)
	}
}

func TestHandleNodeTaskDelete_NotFound(t *testing.T) {
	cfg := &config.Config{}
	cfg.Distributed.Enabled = true
	cfg.Distributed.AdminToken = "admin-token"
	s := &Server{distributed: &DistributedState{NodeTaskQueue: distributed.NewTaskQueue()}, config: cfg}

	req := httptest.NewRequest(http.MethodDelete, "/api/nodes/task/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	w := httptest.NewRecorder()
	s.handleNodeTaskDelete(w, req)

	if w.Code != http.StatusNotFound && w.Code != http.StatusBadRequest {
		t.Fatalf("expected 404 for nonexistent task, got %d", w.Code)
	}
}
