package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/service"
)

func TestHandleWebSocket_ValidationFailure_Returns401(t *testing.T) {
	os.Setenv("UNIMAP_WS_TOKEN", "test-token")
	defer os.Unsetenv("UNIMAP_WS_TOKEN")

	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	s.handleWebSocket(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleWebSocketQuery_WithEngines_SendsQueryStart(t *testing.T) {
	orch := adapter.NewEngineOrchestrator()
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	s := &Server{
		orchestrator: orch,
		queryStatus:  make(map[string]*QueryStatus),
		shutdownCtx:  shutdownCtx,
		service:      service.NewUnifiedService(),
	}
	defer shutdownCancel()

	var messages []map[string]interface{}
	var mu sync.Mutex
	writeJSON := func(v interface{}) error {
		mu.Lock()
		defer mu.Unlock()
		if m, ok := v.(map[string]interface{}); ok {
			messages = append(messages, m)
		}
		return nil
	}

	s.handleWebSocketQuery(nil, map[string]interface{}{"query": "test", "engines": "quake"}, writeJSON)

	mu.Lock()
	msgs := make([]map[string]interface{}, len(messages))
	copy(msgs, messages)
	mu.Unlock()

	// 应该发送 query_start 消息
	if len(msgs) < 1 {
		t.Fatalf("expected at least 1 message, got %d", len(msgs))
	}
	if msgs[0]["type"] != "query_start" {
		t.Fatalf("expected query_start, got %v", msgs[0]["type"])
	}
}

func TestHandleWebSocketQuery_SendsQueryID(t *testing.T) {
	orch := adapter.NewEngineOrchestrator()
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	s := &Server{
		orchestrator: orch,
		queryStatus:  make(map[string]*QueryStatus),
		shutdownCtx:  shutdownCtx,
		service:      service.NewUnifiedService(),
	}
	defer shutdownCancel()

	var messages []map[string]interface{}
	var mu sync.Mutex
	writeJSON := func(v interface{}) error {
		mu.Lock()
		defer mu.Unlock()
		if m, ok := v.(map[string]interface{}); ok {
			messages = append(messages, m)
		}
		return nil
	}

	s.handleWebSocketQuery(nil, map[string]interface{}{"query": "test", "engines": "quake"}, writeJSON)

	mu.Lock()
	msgs := make([]map[string]interface{}, len(messages))
	copy(msgs, messages)
	mu.Unlock()

	// 等待消息写入
	if len(msgs) < 1 {
		t.Fatalf("expected at least 1 message, got %d", len(msgs))
	}
	// 检查 query_id 是否存在
	if _, ok := msgs[0]["query_id"]; !ok {
		t.Fatal("expected query_id in query_start message")
	}
}

func TestHandleWebSocketQuery_QueryIDTracked(t *testing.T) {
	orch := adapter.NewEngineOrchestrator()
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	s := &Server{
		orchestrator: orch,
		queryStatus:  make(map[string]*QueryStatus),
		shutdownCtx:  shutdownCtx,
		service:      service.NewUnifiedService(),
	}
	defer shutdownCancel()

	var messages []map[string]interface{}
	var mu sync.Mutex
	writeJSON := func(v interface{}) error {
		mu.Lock()
		defer mu.Unlock()
		if m, ok := v.(map[string]interface{}); ok {
			messages = append(messages, m)
		}
		return nil
	}

	s.handleWebSocketQuery(nil, map[string]interface{}{"query": "test", "engines": "quake"}, writeJSON)

	mu.Lock()
	msgs := make([]map[string]interface{}, len(messages))
	copy(msgs, messages)
	mu.Unlock()

	// 检查 query_start 消息中包含 query_id
	if len(msgs) < 1 {
		t.Fatalf("expected at least 1 message, got %d", len(msgs))
	}

	var queryID string
	if qid, ok := msgs[0]["query_id"].(string); ok {
		queryID = qid
	}
	if queryID == "" {
		t.Fatal("expected non-empty query_id")
	}

	// 验证查询状态已注册
	s.queryMutex.RLock()
	_, exists := s.queryStatus[queryID]
	s.queryMutex.RUnlock()

	if !exists {
		t.Fatalf("expected query %s to be tracked in queryStatus", queryID)
	}
}

func TestBroadcastMessage_WithConnections(t *testing.T) {
	s := &Server{
		connManager: &ConnectionManager{connections: make(map[string]*managedConn)},
	}

	// broadcast 不应在没有实际连接的情况下 panic
	s.broadcastMessage(map[string]string{"type": "test"})
}

func TestUpdateQueryProgress_ExistingQuery_UpdatesState(t *testing.T) {
	s := &Server{
		connManager: &ConnectionManager{connections: make(map[string]*managedConn)},
		queryStatus: map[string]*QueryStatus{
			"q1": {
				ID:       "q1",
				Query:    "test",
				Engines:  []string{"quake"},
				Status:   "running",
				Progress: 0,
			},
		},
	}

	s.updateQueryProgress("q1", 75.0)

	s.queryMutex.RLock()
	progress := s.queryStatus["q1"].Progress
	s.queryMutex.RUnlock()

	if progress != 75.0 {
		t.Fatalf("expected progress 75.0, got %v", progress)
	}
}

func TestUpdateQueryProgress_NonExistentQuery_NoChange(t *testing.T) {
	s := &Server{
		connManager: &ConnectionManager{connections: make(map[string]*managedConn)},
		queryStatus: make(map[string]*QueryStatus),
	}

	s.updateQueryProgress("nonexistent", 50.0)

	if len(s.queryStatus) != 0 {
		t.Fatalf("expected no queryStatus entries, got %d", len(s.queryStatus))
	}
}

func TestValidateWebSocketRequest_NoToken_DevelopmentMode(t *testing.T) {
	oldToken := os.Getenv("UNIMAP_WS_TOKEN")
	os.Unsetenv("UNIMAP_WS_TOKEN")
	defer func() {
		if oldToken != "" {
			os.Setenv("UNIMAP_WS_TOKEN", oldToken)
		}
	}()

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	if !s.validateWebSocketRequest(req) {
		t.Fatal("expected request to be allowed in development mode")
	}
}

func TestValidateWebSocketRequest_NoToken_ProductionMode(t *testing.T) {
	oldToken := os.Getenv("UNIMAP_WS_TOKEN")
	os.Setenv("UNIMAP_WS_TOKEN", "prod-token")
	defer func() {
		if oldToken != "" {
			os.Setenv("UNIMAP_WS_TOKEN", oldToken)
		} else {
			os.Unsetenv("UNIMAP_WS_TOKEN")
		}
	}()

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	if s.validateWebSocketRequest(req) {
		t.Fatal("expected request to be rejected in production mode without token")
	}
}

func TestHandleWebSocketQuery_PingPongMessages(t *testing.T) {
	var messages []map[string]interface{}
	writeJSON := func(v interface{}) error {
		if m, ok := v.(map[string]interface{}); ok {
			messages = append(messages, m)
		}
		return nil
	}

	orch := adapter.NewEngineOrchestrator()
	s := &Server{
		orchestrator: orch,
		queryStatus:  make(map[string]*QueryStatus),
	}
	s.handleWebSocketQuery(nil, map[string]interface{}{"query": "ping"}, writeJSON)

	// 由于没有引擎，应该返回 query_error
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0]["type"] != "query_error" {
		t.Fatalf("expected query_error for empty engines, got %v", messages[0]["type"])
	}
}

func TestValidateWebSocketRequest_NoEnvToken_Allows(t *testing.T) {
	os.Unsetenv("UNIMAP_WS_TOKEN")
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	if !s.validateWebSocketRequest(req) {
		t.Fatal("expected request to be allowed in development mode")
	}
}

func TestValidateWebSocketRequest_EnvToken_Missing(t *testing.T) {
	os.Setenv("UNIMAP_WS_TOKEN", "test-token")
	defer os.Unsetenv("UNIMAP_WS_TOKEN")

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	if s.validateWebSocketRequest(req) {
		t.Fatal("expected request to be rejected when token is required but missing")
	}
}

func TestValidateWebSocketRequest_EnvToken_Invalid(t *testing.T) {
	os.Setenv("UNIMAP_WS_TOKEN", "test-token")
	defer os.Unsetenv("UNIMAP_WS_TOKEN")

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("X-WebSocket-Token", "wrong-token")
	if s.validateWebSocketRequest(req) {
		t.Fatal("expected request to be rejected with invalid token")
	}
}

func TestValidateWebSocketRequest_EnvToken_ValidHeader(t *testing.T) {
	os.Setenv("UNIMAP_WS_TOKEN", "test-token")
	defer os.Unsetenv("UNIMAP_WS_TOKEN")

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("X-WebSocket-Token", "test-token")
	if !s.validateWebSocketRequest(req) {
		t.Fatal("expected request to be allowed with valid header token")
	}
}

func TestValidateWebSocketRequest_EnvToken_ValidQuery(t *testing.T) {
	os.Setenv("UNIMAP_WS_TOKEN", "test-token")
	defer os.Unsetenv("UNIMAP_WS_TOKEN")

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/ws?token=test-token", nil)
	if !s.validateWebSocketRequest(req) {
		t.Fatal("expected request to be allowed with valid query token")
	}
}

func TestParseWSStringList(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected []string
	}{
		{"nil", nil, nil},
		{"not string", 42, nil},
		{"empty string", "", nil},
		{"single value", "quake", []string{"quake"}},
		{"comma separated", "quake,fofa,hunter", []string{"quake", "fofa", "hunter"}},
		{"with spaces", "quake, fofa , hunter", []string{"quake", "fofa", "hunter"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWSStringList(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d items, got %d", len(tt.expected), len(got))
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Fatalf("item %d: expected %q, got %q", i, tt.expected[i], got[i])
				}
			}
		})
	}
}

func TestParseWSInt(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		def      int
		expected int
	}{
		{"nil uses default", nil, 50, 50},
		{"not int uses default", "abc", 50, 50},
		{"valid int", 100, 50, 100},
		{"float64", float64(25), 50, 25},
		{"zero returns default", 0, 50, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWSInt(tt.input, tt.def)
			if got != tt.expected {
				t.Fatalf("expected %d, got %d", tt.expected, got)
			}
		})
	}
}

func TestParseWSBool(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected bool
	}{
		{"nil", nil, false},
		{"true string", "true", true},
		{"false string", "false", false},
		{"true bool", true, true},
		{"false bool", false, false},
		{"1 string", "1", true},
		{"yes string", "yes", true},
		{"random string", "random", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWSBool(tt.input)
			if got != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestHandleWebSocketQuery_EmptyQuery_ReturnsError(t *testing.T) {
	var messages []map[string]interface{}
	writeJSON := func(v interface{}) error {
		if m, ok := v.(map[string]interface{}); ok {
			messages = append(messages, m)
		}
		return nil
	}

	s := &Server{}
	s.handleWebSocketQuery(nil, map[string]interface{}{"query": ""}, writeJSON)

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0]["type"] != "query_error" {
		t.Fatalf("expected query_error, got %v", messages[0]["type"])
	}
}

func TestHandleWebSocketQuery_NoEngines_ReturnsError(t *testing.T) {
	var messages []map[string]interface{}
	writeJSON := func(v interface{}) error {
		if m, ok := v.(map[string]interface{}); ok {
			messages = append(messages, m)
		}
		return nil
	}

	s := &Server{
		orchestrator: adapter.NewEngineOrchestrator(),
	}
	s.handleWebSocketQuery(nil, map[string]interface{}{"query": "country=\"CN\""}, writeJSON)

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0]["type"] != "query_error" {
		t.Fatalf("expected query_error, got %v", messages[0]["type"])
	}
}

func TestBroadcastMessage_NilConnections_NoPanic(t *testing.T) {
	s := &Server{
		connManager: &ConnectionManager{connections: make(map[string]*managedConn)},
	}
	s.broadcastMessage(map[string]string{"type": "ping"})
}

func TestUpdateQueryProgress_NonExistentQuery_NoBroadcast(t *testing.T) {
	s := &Server{
		connManager: &ConnectionManager{connections: make(map[string]*managedConn)},
		queryStatus: make(map[string]*QueryStatus),
	}
	s.updateQueryProgress("nonexistent", 50.0)
}

func TestValidateQueryInput(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"empty", "", true},
		{"whitespace", "   ", true},
		{"valid", "country=\"CN\"", false},
		{"valid with engine", "engine:quake ip=1.1.1.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateQueryInput(tt.query)
			if (err != nil) != tt.wantErr {
				t.Fatalf("expected error=%v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestParseBoolValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"empty", "", false},
		{"true", "true", true},
		{"false", "false", false},
		{"1", "1", true},
		{"0", "0", false},
		{"yes", "yes", true},
		{"no", "no", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseBoolValue(tt.input)
			if got != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestAppendUniqueStrings(t *testing.T) {
	tests := []struct {
		name     string
		base     []string
		extra    []string
		expected []string
	}{
		{"both empty", nil, nil, nil},
		{"base only", []string{"a"}, nil, []string{"a"}},
		{"extra only", nil, []string{"b"}, []string{"b"}},
		{"no duplicates", []string{"a"}, []string{"b"}, []string{"a", "b"}},
		{"with duplicates", []string{"a", "b"}, []string{"b", "c"}, []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendUniqueStrings(tt.base, tt.extra)
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d items, got %d: %v", len(tt.expected), len(got), got)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Fatalf("item %d: expected %q, got %q", i, tt.expected[i], got[i])
				}
			}
		})
	}
}

func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", "****"},
		{"short", "ab", "****"},
		{"eight chars", "12345678", "****"},
		{"nine chars", "123456789", "1234****6789"},
		{"long", "1234567890abcdef", "1234****cdef"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskAPIKey(tt.input)
			if got != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}
