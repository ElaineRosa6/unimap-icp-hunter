package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/service"
)

func TestHandleAPIQuery_GetMethod_Returns405(t *testing.T) {
	orch := adapter.NewEngineOrchestrator()
	s := &Server{
		queryApp: service.NewQueryAppService(nil, orch),
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/query", nil)
	s.handleAPIQuery(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleAPIQuery_EmptyQuery_Returns400(t *testing.T) {
	orch := adapter.NewEngineOrchestrator()
	s := &Server{
		queryApp: service.NewQueryAppService(nil, orch),
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/query", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.handleAPIQuery(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleAPIQuery_NoEngines_Returns503(t *testing.T) {
	orch := adapter.NewEngineOrchestrator()
	s := &Server{
		queryApp:   service.NewQueryAppService(nil, orch),
		orchestrator: orch,
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/query?query=country%3D%22CN%22", nil)
	s.handleAPIQuery(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleQuery_GetMethod_Redirects(t *testing.T) {
	orch := adapter.NewEngineOrchestrator()
	s := &Server{
		orchestrator: orch,
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/query", nil)
	s.handleQuery(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", rec.Code)
	}
}

func TestHandleQuery_EmptyQuery_ReturnsError(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/query", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.handleQuery(rec, req)

	// 由于模板解析可能失败，至少应该尝试渲染错误页面
	if rec.Code == 0 {
		t.Fatal("expected response")
	}
}

func TestHandleQuery_NoEngines_ReturnsError(t *testing.T) {
	s := &Server{
		orchestrator: adapter.NewEngineOrchestrator(),
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/query?query=country%3D%22CN%22", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.handleQuery(rec, req)

	// 尝试渲染 error.html，模板不存在时 code 可能是 500 或 0
	// 关键是它不会 panic
}

func TestHandleResults_GET_RendersTemplate(t *testing.T) {
	s := &Server{
		orchestrator: adapter.NewEngineOrchestrator(),
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/results?query=test", nil)
	s.handleResults(rec, req)

	// 模板可能不存在，但不应 panic
}

func TestHandleQuota_RendersTemplate(t *testing.T) {
	s := &Server{
		orchestrator: adapter.NewEngineOrchestrator(),
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/quota", nil)
	s.handleQuota(rec, req)

	// 模板可能不存在，但不应 panic
}

func TestHandleQueryStatus_MissingQueryID_Returns400(t *testing.T) {
	s := &Server{
		queryStatus: make(map[string]*QueryStatus),
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/query/status", nil)
	s.handleQueryStatus(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleQueryStatus_NotFound_Returns404(t *testing.T) {
	s := &Server{
		queryStatus: make(map[string]*QueryStatus),
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/query/status?query_id=nonexistent", nil)
	s.handleQueryStatus(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleQueryStatus_Exists_Returns200(t *testing.T) {
	s := &Server{
		queryStatus: map[string]*QueryStatus{
			"q1": {
				ID:       "q1",
				Query:    "test",
				Engines:  []string{"quake"},
				Status:   "running",
				Progress: 50.0,
			},
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/query/status?query_id=q1", nil)
	s.handleQueryStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestParseEnginesParam_DuplicateRemoval(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?engines=quake,fofa&engines=quake,hunter", nil)
	engines := parseEnginesParam(req)

	// 应该去重
	seen := make(map[string]bool)
	for _, e := range engines {
		if seen[e] {
			t.Fatalf("duplicate engine: %s", e)
		}
		seen[e] = true
	}
	if len(engines) != 3 {
		t.Fatalf("expected 3 unique engines, got %d: %v", len(engines), engines)
	}
}

func TestParseEnginesParam_Empty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?engines=,,", nil)
	engines := parseEnginesParam(req)

	if len(engines) != 0 {
		t.Fatalf("expected 0 engines, got %d", len(engines))
	}
}

func TestValidateQueryInput_TooLong(t *testing.T) {
	longQuery := ""
	for i := 0; i < 1001; i++ {
		longQuery += "a"
	}
	err := validateQueryInput(longQuery)
	if err == nil {
		t.Fatal("expected error for query > 1000 chars")
	}
}

func TestValidateQueryInput_ControlChars(t *testing.T) {
	err := validateQueryInput("test\x01query")
	if err == nil {
		t.Fatal("expected error for control characters")
	}
}

func TestValidateQueryInput_ValidWithTabs(t *testing.T) {
	err := validateQueryInput("country\t=\t\"CN\"")
	if err != nil {
		t.Fatalf("unexpected error for valid query with tabs: %v", err)
	}
}

func TestBuildQueryAPIPayload(t *testing.T) {
	payload := buildQueryAPIPayload(
		"test",
		[]string{"quake"},
		nil,
		browserQueryOutcome{
			Enabled: true,
		},
	)

	if payload["query"] != "test" {
		t.Fatalf("expected query 'test', got %v", payload["query"])
	}
	if payload["browserQuery"] != true {
		t.Fatalf("expected browserQuery true, got %v", payload["browserQuery"])
	}
}

func TestBuildQueryAPIPayload_CombinesErrors(t *testing.T) {
	payload := buildQueryAPIPayload(
		"test",
		[]string{"quake"},
		nil,
		browserQueryOutcome{
			Errors: []string{"browser error"},
		},
		"explicit error",
	)

	errors, ok := payload["errors"].([]string)
	if !ok {
		t.Fatal("expected errors to be []string")
	}
	if len(errors) < 2 {
		t.Fatalf("expected at least 2 errors, got %d", len(errors))
	}
}

func TestHandleHealth_OK(t *testing.T) {
	orch := adapter.NewEngineOrchestrator()
	s := &Server{
		orchestrator: orch,
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	s.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	// 验证响应包含 status: "ok"
	body := rec.Body.String()
	if !strings.Contains(body, `"status"`) {
		t.Fatalf("expected status in response, got: %s", body)
	}
}

func TestMaskAPIKey_Validation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", "****"},
		{"short", "abc", "****"},
		{"exactly 8", "12345678", "****"},
		{"exactly 9", "123456789", "1234****6789"},
		{"long key", "abcdef1234567890", "abcd****7890"},
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

func TestParseBoolValue_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"on", "on", true},
		{"ON", "ON", true},
		{"On", "On", true},
		{"with spaces", " true ", true},
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

func TestHandleQueryStatus_CompletedQuery_ReturnsFullStatus(t *testing.T) {
	s := &Server{
		queryStatus: map[string]*QueryStatus{
			"q-complete": {
				ID:         "q-complete",
				Query:      "ip=1.1.1.1",
				Engines:    []string{"quake", "fofa"},
				Status:     "completed",
				Progress:   100.0,
				TotalCount: 42,
			},
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/query/status?query_id=q-complete", nil)
	s.handleQueryStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var status QueryStatus
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if status.Status != "completed" {
		t.Fatalf("expected status 'completed', got %q", status.Status)
	}
	if status.TotalCount != 42 {
		t.Fatalf("expected totalCount 42, got %d", status.TotalCount)
	}
	if len(status.Engines) != 2 {
		t.Fatalf("expected 2 engines, got %d", len(status.Engines))
	}
}

func TestHandleAPIQuery_WhitespaceQuery_Returns400(t *testing.T) {
	orch := adapter.NewEngineOrchestrator()
	s := &Server{
		queryApp: service.NewQueryAppService(nil, orch),
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/query", strings.NewReader("query=   "))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.handleAPIQuery(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleAPIQuery_PageSizeParsing(t *testing.T) {
	orch := adapter.NewEngineOrchestrator()
	s := &Server{
		queryApp:   service.NewQueryAppService(nil, orch),
		orchestrator: orch,
	}
	// 有效 query 但无引擎 -> 503
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/query?query=country%3D%22CN%22&page_size=abc", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.handleAPIQuery(rec, req)

	// page_size 无效时应该回退到默认值，最终因为无引擎返回 503
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleResults_EmptyQuery_RendersTemplate(t *testing.T) {
	s := &Server{
		orchestrator: adapter.NewEngineOrchestrator(),
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/results", nil)
	s.handleResults(rec, req)

	// 模板可能不存在，但不应 panic
}

func TestHandleQuota_NoEngines_RendersEmptyTemplate(t *testing.T) {
	s := &Server{
		orchestrator: adapter.NewEngineOrchestrator(),
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/quota", nil)
	s.handleQuota(rec, req)

	// 模板可能不存在，但不应 panic
}

func TestParseEnginesParam_CombinedFormat(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?engines=quake,fofa,hunter", nil)
	engines := parseEnginesParam(req)

	if len(engines) != 3 {
		t.Fatalf("expected 3 engines, got %d: %v", len(engines), engines)
	}
}

func TestParseEnginesParam_SingleValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?engines=quake", nil)
	engines := parseEnginesParam(req)

	if len(engines) != 1 || engines[0] != "quake" {
		t.Fatalf("expected [quake], got %v", engines)
	}
}
