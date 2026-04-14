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

func TestNodeRegisterHeartbeatStatus(t *testing.T) {
	cfg := &config.Config{}
	cfg.Distributed.Enabled = true
	cfg.Distributed.AdminToken = "test-admin-token"
	cfg.Distributed.NodeAuthTokens = map[string]string{"node-a": "node-token-a"}
	s := &Server{
		distributed: &DistributedState{NodeRegistry: distributed.NewRegistry(60 * time.Second)},
		config:      cfg,
	}

	registerBody := map[string]interface{}{
		"node_id":         "node-a",
		"hostname":        "worker-a",
		"region":          "cn-east",
		"max_concurrency": 3,
		"capabilities":    []string{"port_scan", "screenshot"},
	}
	registerBytes, _ := json.Marshal(registerBody)
	registerReq := httptest.NewRequest(http.MethodPost, "/api/nodes/register", bytes.NewReader(registerBytes))
	registerReq.Header.Set("Authorization", "Bearer node-token-a")
	registerW := httptest.NewRecorder()
	s.handleNodeRegister(registerW, registerReq)
	if registerW.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", registerW.Code, registerW.Body.String())
	}

	heartbeatBody := map[string]interface{}{
		"node_id":         "node-a",
		"current_load":    1,
		"max_concurrency": 3,
		"avg_latency_ms":  12.5,
		"success_rate_5m": 99.1,
	}
	heartbeatBytes, _ := json.Marshal(heartbeatBody)
	heartbeatReq := httptest.NewRequest(http.MethodPost, "/api/nodes/heartbeat", bytes.NewReader(heartbeatBytes))
	heartbeatReq.Header.Set("Authorization", "Bearer node-token-a")
	heartbeatW := httptest.NewRecorder()
	s.handleNodeHeartbeat(heartbeatW, heartbeatReq)
	if heartbeatW.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", heartbeatW.Code, heartbeatW.Body.String())
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/nodes/status", nil)
	statusReq.Header.Set("Authorization", "Bearer test-admin-token")
	statusW := httptest.NewRecorder()
	s.handleNodeStatus(statusW, statusReq)
	if statusW.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", statusW.Code, statusW.Body.String())
	}

	var statusResp struct {
		Success bool `json:"success"`
		Summary struct {
			Total   int `json:"total"`
			Online  int `json:"online"`
			Offline int `json:"offline"`
		} `json:"summary"`
		Nodes []struct {
			NodeID         string `json:"node_id"`
			CurrentLoad    int    `json:"current_load"`
			MaxConcurrency int    `json:"max_concurrency"`
			Online         bool   `json:"online"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(statusW.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("unmarshal status failed: %v", err)
	}
	if !statusResp.Success {
		t.Fatalf("expected success=true")
	}
	if statusResp.Summary.Total != 1 || statusResp.Summary.Online != 1 {
		t.Fatalf("unexpected summary: %+v", statusResp.Summary)
	}
	if len(statusResp.Nodes) != 1 || statusResp.Nodes[0].NodeID != "node-a" {
		t.Fatalf("unexpected nodes: %+v", statusResp.Nodes)
	}
	if statusResp.Nodes[0].CurrentLoad != 1 || statusResp.Nodes[0].MaxConcurrency != 3 || !statusResp.Nodes[0].Online {
		t.Fatalf("unexpected node state: %+v", statusResp.Nodes[0])
	}
}

func TestNodeRegisterValidation(t *testing.T) {
	s := &Server{
		distributed: &DistributedState{NodeRegistry: distributed.NewRegistry(60 * time.Second)},
		config:      &config.Config{},
	}
	s.config.Distributed.Enabled = true

	registerBody := map[string]interface{}{"hostname": "worker-a"}
	registerBytes, _ := json.Marshal(registerBody)
	registerReq := httptest.NewRequest(http.MethodPost, "/api/nodes/register", bytes.NewReader(registerBytes))
	registerW := httptest.NewRecorder()
	s.handleNodeRegister(registerW, registerReq)
	if registerW.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", registerW.Code)
	}
}

func TestNodeEndpoints_DistributedDisabled(t *testing.T) {
	s := &Server{distributed: &DistributedState{NodeRegistry: distributed.NewRegistry(60 * time.Second)}, config: &config.Config{}}

	registerBody := map[string]interface{}{"node_id": "node-a"}
	registerBytes, _ := json.Marshal(registerBody)
	registerReq := httptest.NewRequest(http.MethodPost, "/api/nodes/register", bytes.NewReader(registerBytes))
	registerW := httptest.NewRecorder()
	s.handleNodeRegister(registerW, registerReq)
	if registerW.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when distributed disabled, got %d", registerW.Code)
	}
}

func TestNodeNetworkProfile(t *testing.T) {
	cfg := &config.Config{}
	cfg.Distributed.Enabled = true
	cfg.Distributed.AdminToken = "test-admin-token"
	s := &Server{distributed: &DistributedState{NodeRegistry: distributed.NewRegistry(60 * time.Second)}, config: cfg}

	_, _ = s.distributed.NodeRegistry.Register(distributed.NodeRegistration{NodeID: "node-a", Region: "cn-east", EgressIP: "1.2.3.4", MaxConcurrency: 3})
	_, _ = s.distributed.NodeRegistry.Heartbeat(distributed.NodeHeartbeat{NodeID: "node-a", CurrentLoad: 1, AvgLatencyMS: 11.2, SuccessRate5m: 98.7})

	req := httptest.NewRequest(http.MethodGet, "/api/nodes/network/profile", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")
	w := httptest.NewRecorder()
	s.handleNodeNetworkProfile(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Success bool `json:"success"`
		Summary struct {
			Total int `json:"total"`
		} `json:"summary"`
		Profiles []map[string]interface{} `json:"profiles"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if !resp.Success || resp.Summary.Total != 1 || len(resp.Profiles) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestNodeRegister_NodeAuthToken(t *testing.T) {
	cfg := &config.Config{}
	cfg.Distributed.Enabled = true
	cfg.Distributed.NodeAuthTokens = map[string]string{"node-a": "token-a"}
	s := &Server{distributed: &DistributedState{NodeRegistry: distributed.NewRegistry(60 * time.Second)}, config: cfg}

	body := map[string]interface{}{"node_id": "node-a", "hostname": "worker-a"}
	b, _ := json.Marshal(body)

	unauthReq := httptest.NewRequest(http.MethodPost, "/api/nodes/register", bytes.NewReader(b))
	unauthW := httptest.NewRecorder()
	s.handleNodeRegister(unauthW, unauthReq)
	if unauthW.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d, body=%s", unauthW.Code, unauthW.Body.String())
	}

	authReq := httptest.NewRequest(http.MethodPost, "/api/nodes/register", bytes.NewReader(b))
	authReq.Header.Set("Authorization", "Bearer token-a")
	authW := httptest.NewRecorder()
	s.handleNodeRegister(authW, authReq)
	if authW.Code != http.StatusOK {
		t.Fatalf("expected 200 with token, got %d, body=%s", authW.Code, authW.Body.String())
	}
}

func TestNodeStatus_AdminToken(t *testing.T) {
	cfg := &config.Config{}
	cfg.Distributed.Enabled = true
	cfg.Distributed.AdminToken = "admin-token"
	s := &Server{distributed: &DistributedState{NodeRegistry: distributed.NewRegistry(60 * time.Second)}, config: cfg}

	unauthReq := httptest.NewRequest(http.MethodGet, "/api/nodes/status", nil)
	unauthW := httptest.NewRecorder()
	s.handleNodeStatus(unauthW, unauthReq)
	if unauthW.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without admin token, got %d", unauthW.Code)
	}

	authReq := httptest.NewRequest(http.MethodGet, "/api/nodes/status", nil)
	authReq.Header.Set("Authorization", "Bearer admin-token")
	authW := httptest.NewRecorder()
	s.handleNodeStatus(authW, authReq)
	if authW.Code != http.StatusOK {
		t.Fatalf("expected 200 with admin token, got %d, body=%s", authW.Code, authW.Body.String())
	}
}

func TestNodeGet_AdminToken(t *testing.T) {
	cfg := &config.Config{}
	cfg.Distributed.Enabled = true
	cfg.Distributed.AdminToken = "admin-token"
	s := &Server{distributed: &DistributedState{NodeRegistry: distributed.NewRegistry(60 * time.Second)}, config: cfg}

	_, _ = s.distributed.NodeRegistry.Register(distributed.NodeRegistration{NodeID: "node-a", Hostname: "worker-a"})

	unauthReq := httptest.NewRequest(http.MethodGet, "/api/nodes/get?node_id=node-a", nil)
	unauthW := httptest.NewRecorder()
	s.handleNodeGet(unauthW, unauthReq)
	if unauthW.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without admin token, got %d", unauthW.Code)
	}

	authReq := httptest.NewRequest(http.MethodGet, "/api/nodes/get?node_id=node-a", nil)
	authReq.Header.Set("Authorization", "Bearer admin-token")
	authW := httptest.NewRecorder()
	s.handleNodeGet(authW, authReq)
	if authW.Code != http.StatusOK {
		t.Fatalf("expected 200 with admin token, got %d, body=%s", authW.Code, authW.Body.String())
	}
}

func TestNodeDeregister_AdminTokenFallback(t *testing.T) {
	cfg := &config.Config{}
	cfg.Distributed.Enabled = true
	cfg.Distributed.AdminToken = "admin-token"
	cfg.Distributed.NodeAuthTokens = map[string]string{"node-a": "token-a"}
	s := &Server{distributed: &DistributedState{NodeRegistry: distributed.NewRegistry(60 * time.Second)}, config: cfg}

	_, _ = s.distributed.NodeRegistry.Register(distributed.NodeRegistration{NodeID: "node-a", Hostname: "worker-a"})

	unauthReq := httptest.NewRequest(http.MethodDelete, "/api/nodes/deregister?node_id=node-a", nil)
	unauthW := httptest.NewRecorder()
	s.handleNodeDeregister(unauthW, unauthReq)
	if unauthW.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", unauthW.Code)
	}

	authReq := httptest.NewRequest(http.MethodDelete, "/api/nodes/deregister?node_id=node-a", nil)
	authReq.Header.Set("Authorization", "Bearer admin-token")
	authW := httptest.NewRecorder()
	s.handleNodeDeregister(authW, authReq)
	if authW.Code != http.StatusOK {
		t.Fatalf("expected 200 with admin token, got %d, body=%s", authW.Code, authW.Body.String())
	}
}
