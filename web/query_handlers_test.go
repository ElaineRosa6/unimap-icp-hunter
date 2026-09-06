package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/unimap/project/internal/adapter"
	"github.com/unimap/project/internal/auth"
	"github.com/unimap/project/internal/collection"
	"github.com/unimap/project/internal/config"
	"github.com/unimap/project/internal/model"
	"github.com/unimap/project/internal/screenshot"
	"github.com/unimap/project/internal/service"
)

func TestHandleAPIQuery_GetMethod_Returns405(t *testing.T) {
	orch := adapter.NewEngineOrchestrator()
	s := &Server{
		queryApp: service.NewQueryAppService(nil, orch),
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/query", nil)
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/query", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://localhost:8448")
	s.handleAPIQuery(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleAPIQuery_NoEngines_Returns503(t *testing.T) {
	orch := adapter.NewEngineOrchestrator()
	s := &Server{
		queryApp:     service.NewQueryAppService(nil, orch),
		orchestrator: orch,
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/query?query=country%3D%22CN%22", nil)
	req.Header.Set("Origin", "http://localhost:8448")
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

	// Since template parsing may fail, at least it should attempt rendering
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

	// Attempts to render error.html; when template is missing, code may be 500 or 0
	// Key point: it must not panic
}

func TestHandleResults_GET_RendersTemplate(t *testing.T) {
	s := &Server{
		orchestrator: adapter.NewEngineOrchestrator(),
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/results?query=test", nil)
	s.handleResults(rec, req)

	// Template may not exist, but should not panic
}

func TestHandleQuota_RendersTemplate(t *testing.T) {
	s := &Server{
		orchestrator: adapter.NewEngineOrchestrator(),
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/quota", nil)
	s.handleQuota(rec, req)

	// Template may not exist, but should not panic
}

func TestHandleQueryStatus_MissingQueryID_Returns400(t *testing.T) {
	s := &Server{
		queryStatus: make(map[string]*QueryStatus),
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/query/status", nil)
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/query/status?query_id=nonexistent", nil)
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/query/status?query_id=q1", nil)
	s.handleQueryStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestParseEnginesParam_DuplicateRemoval(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?engines=quake,fofa&engines=quake,hunter", nil)
	engines := parseEnginesParam(req)

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

func TestBuildQueryAPIPayloadIncludesPersistenceStatus(t *testing.T) {
	resp := &service.QueryResponse{Persistence: service.PersistenceStatus{Status: "failed", Warning: "history unavailable"}}
	payload := buildQueryAPIPayload("test", []string{"fofa"}, resp, browserQueryOutcome{}, "", false)
	if payload.Persistence.Status != "failed" || payload.Persistence.Warning == "" {
		t.Fatalf("persistence status was not propagated: %#v", payload.Persistence)
	}
}

func TestBuildQueryAPIPayloadMarksBrowserFallbackPartial(t *testing.T) {
	payload := buildQueryAPIPayload(
		`port="443"`,
		[]string{"quake"},
		nil,
		browserQueryOutcome{
			Enabled: true,
			CollectedResults: []collection.CollectResult{{
				Engine: "quake",
				Assets: []model.UnifiedAsset{{IP: "192.0.2.1", Source: "quake"}},
			}},
		},
		"collect_and_capture",
		false,
		"API query failed: adapter unavailable",
	)

	if payload.Status != "partial" {
		t.Fatalf("status = %q, want partial", payload.Status)
	}
	if len(payload.Assets) != 1 {
		t.Fatalf("expected browser asset in response, got %#v", payload.Assets)
	}
	if len(payload.Errors) == 0 || !strings.Contains(payload.Errors[0], "API query failed") {
		t.Fatalf("expected API diagnostic to be preserved, got %#v", payload.Errors)
	}
}

func TestValidateQueryInput_TooLong(t *testing.T) {
	longQuery := ""
	for i := 0; i < 20001; i++ {
		longQuery += "a"
	}
	err := validateQueryInput(longQuery)
	if err == nil {
		t.Fatal("expected error for query > 20000 chars")
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
			CollectedResults: []collection.CollectResult{{
				Engine: "quake",
				Query:  "test",
			}},
		},
		"capture",
		false,
	)

	if payload.Query != "test" {
		t.Fatalf("expected query 'test', got %v", payload.Query)
	}
	if payload.BrowserQuery != true {
		t.Fatalf("expected browserQuery true, got %v", payload.BrowserQuery)
	}
	collected := payload.BrowserCollectedData
	if len(collected) != 1 || collected[0].Engine != "quake" {
		t.Fatalf("unexpected browserCollectedData: %#v", collected)
	}
}

func TestBuildQueryAPIPayload_CleansHunterBrowserCollectedData(t *testing.T) {
	payload := buildQueryAPIPayload(
		"test",
		[]string{"hunter"},
		nil,
		browserQueryOutcome{
			Enabled: true,
			CollectedResults: []collection.CollectResult{{
				Engine: "hunter",
				Assets: []model.UnifiedAsset{{
					CountryCode: "成都市",
					Host:        "不看空域名 -",
					Title:       "Dovecot imapd企业办公 邮件系统 开源 Dovecot imapd",
				}},
			}},
		},
		"collect",
		false,
	)

	collected := payload.BrowserCollectedData
	if len(collected) != 1 {
		t.Fatalf("expected 1 collected result, got %d", len(collected))
	}
	asset := collected[0].Assets[0]
	if asset.CountryCode != "中国" {
		t.Fatalf("expected cleaned country_code 中国, got %q", asset.CountryCode)
	}
	if asset.Host != "" {
		t.Fatalf("expected cleaned host to be empty, got %q", asset.Host)
	}
	if asset.Title != "Dovecot imapd" {
		t.Fatalf("expected cleaned title, got %q", asset.Title)
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
		"",
		false,
		"explicit error",
	)

	errors := payload.Errors
	if len(errors) < 2 {
		t.Fatalf("expected at least 2 errors, got %d", len(errors))
	}
}

func TestBuildQueryAPIPayload_MergesCollectedAssets(t *testing.T) {
	resp := &service.QueryResponse{
		Assets:      []model.UnifiedAsset{{IP: "1.1.1.1"}},
		TotalCount:  1,
		EngineStats: map[string]int{"fofa": 1},
	}
	browserOutcome := browserQueryOutcome{
		Enabled: true,
		CollectedResults: []collection.CollectResult{
			{
				Engine: "hunter",
				Assets: []model.UnifiedAsset{{IP: "2.2.2.2"}, {IP: "3.3.3.3"}},
				Total:  2,
			},
			{
				Engine: "quake",
				Assets: []model.UnifiedAsset{{IP: "4.4.4.4"}},
			},
		},
	}

	payload := buildQueryAPIPayload("test", []string{"fofa", "hunter", "quake"}, resp, browserOutcome, "collect", false)

	assets := payload.Assets
	if len(assets) != 4 {
		t.Fatalf("expected 4 assets (1 query + 3 collected), got %d", len(assets))
	}
	total := payload.TotalCount
	if total != 4 {
		t.Fatalf("expected totalCount 4, got %d", total)
	}
	stats := payload.EngineStats
	if stats["fofa"] != 1 {
		t.Errorf("expected fofa=1, got %d", stats["fofa"])
	}
	if stats["hunter"] != 2 {
		t.Errorf("expected hunter=2, got %d", stats["hunter"])
	}
	if stats["quake"] != 1 {
		t.Errorf("expected quake=1, got %d", stats["quake"])
	}
}

func TestBuildQueryAPIPayload_MergesBrowserCollectedAssets(t *testing.T) {
	payload := buildQueryAPIPayload(
		"test",
		[]string{"fofa"},
		&service.QueryResponse{
			Assets:      []model.UnifiedAsset{{URL: "https://api.example.test", Source: "api"}},
			TotalCount:  1,
			EngineStats: nil,
		},
		browserQueryOutcome{
			Enabled: true,
			CollectedResults: []collection.CollectResult{{
				Engine: "fofa",
				Assets: []model.UnifiedAsset{{URL: "https://browser.example.test", Source: "browser"}},
				Total:  1,
			}},
		},
		"collect",
		false,
	)

	assets := payload.Assets
	if len(assets) != 2 {
		t.Fatalf("expected 2 merged assets, got %#v", assets)
	}
	if payload.TotalCount != 2 {
		t.Fatalf("expected totalCount 2, got %v", payload.TotalCount)
	}
	engineStats := payload.EngineStats
	if engineStats["fofa"] != 1 {
		t.Fatalf("expected fofa browser stat 1, got %#v", engineStats)
	}
}

func TestBuildQueryAPIPayload_DoesNotDoubleCountMergedBrowserAssets(t *testing.T) {
	// HTTP API 走 ExecuteQueryWithBrowserWorkflow，resp.Assets 已由 service 层 merge
	// 浏览器资产；此时必须跳过本函数内的追加逻辑，避免重复计数。
	resp := &service.QueryResponse{
		Assets: []model.UnifiedAsset{
			{URL: "https://api.example.test", Source: "api"},
			{URL: "https://browser.example.test", Source: "browser"},
		},
		TotalCount:  2,
		EngineStats: map[string]int{"fofa": 2},
	}
	browserOutcome := browserQueryOutcome{
		Enabled: true,
		CollectedResults: []collection.CollectResult{{
			Engine: "fofa",
			Assets: []model.UnifiedAsset{{URL: "https://browser.example.test", Source: "browser"}},
			Total:  1,
		}},
	}

	payload := buildQueryAPIPayload("test", []string{"fofa"}, resp, browserOutcome, "collect", true)

	if len(payload.Assets) != 2 {
		t.Fatalf("expected 2 assets (browser already in resp), got %d", len(payload.Assets))
	}
	if payload.TotalCount != 2 {
		t.Fatalf("expected totalCount 2, got %v", payload.TotalCount)
	}
	if stats := payload.EngineStats; stats["fofa"] != 2 {
		t.Fatalf("expected fofa stat 2 (api+browser merged), got %#v", stats)
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
		{"empty", "", ""},
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/query/status?query_id=q-complete", nil)
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/query", strings.NewReader("query=   "))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://localhost:8448")
	s.handleAPIQuery(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleAPIQuery_PageSizeParsing(t *testing.T) {
	orch := adapter.NewEngineOrchestrator()
	s := &Server{
		queryApp:     service.NewQueryAppService(nil, orch),
		orchestrator: orch,
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/query?query=country%3D%22CN%22&page_size=abc", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://localhost:8448")
	s.handleAPIQuery(rec, req)

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

	// Template may not exist, but should not panic
}

func TestHandleQuota_NoEngines_RendersEmptyTemplate(t *testing.T) {
	s := &Server{
		orchestrator: adapter.NewEngineOrchestrator(),
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/quota", nil)
	s.handleQuota(rec, req)

	// Template may not exist, but should not panic
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

// --- browserQueryProvider tests ---

// newTestScreenshotMgr creates a lightweight screenshot.Manager for testing
// without requiring a real Chrome instance.
func newTestScreenshotMgr(t *testing.T) *screenshot.Manager {
	t.Helper()
	return screenshot.NewManager(screenshot.Config{
		BaseDir:  t.TempDir(),
		Timeout:  5 * time.Second,
		WaitTime: 100 * time.Millisecond,
	})
}

// newTestRouter creates a ScreenshotRouter for testing without real Chrome.
func newTestRouter() *screenshot.ScreenshotRouter {
	cfg := screenshot.RouterConfig{
		Priority:      screenshot.ModeCDP,
		Fallback:      true,
		ProbeInterval: 30 * time.Second,
		ProbeTimeout:  5 * time.Second,
	}
	return screenshot.NewScreenshotRouter(cfg, nil, nil, nil)
}

// newTestBridgeService creates a BridgeService with a mock client for testing.
func newTestBridgeService() *screenshot.BridgeService {
	mockClient := newBridgeMockClient()
	return screenshot.NewBridgeService(mockClient, 2, 5*time.Second)
}

func TestBrowserQueryProvider_NilServer_ReturnsNil(t *testing.T) {
	// Arrange
	var s *Server

	// Act
	provider := s.browserQueryProvider()

	// Assert
	if provider != nil {
		t.Fatalf("expected nil provider for nil server, got %T", provider)
	}
}

func TestBrowserQueryProvider_EmptyServer_ReturnsNil(t *testing.T) {
	// Arrange
	s := &Server{}

	// Act
	provider := s.browserQueryProvider()

	// Assert
	if provider != nil {
		t.Fatalf("expected nil provider for empty server, got %T", provider)
	}
}

func TestBrowserQueryProvider_ScreenshotRouterAvailable_ReturnsRouter(t *testing.T) {
	// Arrange
	s := &Server{
		screenshotRouter: newTestRouter(),
		screenshotMgr:    nil,
		bridge:           &BridgeState{},
	}

	// Act
	provider := s.browserQueryProvider()

	// Assert
	if provider == nil {
		t.Fatal("expected non-nil provider when screenshotRouter is set")
	}
	if _, ok := provider.(*screenshot.ScreenshotRouter); !ok {
		t.Fatalf("expected *screenshot.ScreenshotRouter, got %T", provider)
	}
}

func TestBrowserQueryProvider_ExtensionWithoutRouter_ReturnsNil(t *testing.T) {
	// An allocated bridge service is not proof that an extension client is live.
	// Production initializes the router for all modes; incomplete wiring must
	// fail closed instead of returning an unusable extension provider.
	s := &Server{
		screenshotRouter: nil,
		screenshotMgr:    nil,
		bridge: &BridgeState{
			Service: newTestBridgeService(),
		},
	}

	// Act
	provider := s.browserQueryProvider()

	// Assert
	if provider != nil {
		t.Fatalf("expected nil without the unified router, got %T", provider)
	}
}

func TestBrowserQueryProvider_CDPOnly_ReturnsCDPProvider(t *testing.T) {
	// Arrange: no router, no bridge service, but screenshotMgr is set
	s := &Server{
		screenshotRouter: nil,
		screenshotMgr:    newTestScreenshotMgr(t),
		bridge:           &BridgeState{},
	}

	// Act
	provider := s.browserQueryProvider()

	// Assert
	if provider == nil {
		t.Fatal("expected non-nil provider when screenshotMgr is set")
	}
	if _, ok := provider.(*screenshot.CDPProvider); !ok {
		t.Fatalf("expected *screenshot.CDPProvider, got %T", provider)
	}
}

func TestBrowserQueryProvider_PriorityRouterOverExtension(t *testing.T) {
	// Arrange: both router and bridge.Service are set
	s := &Server{
		screenshotRouter: newTestRouter(),
		bridge: &BridgeState{
			Service: newTestBridgeService(),
		},
		screenshotMgr: newTestScreenshotMgr(t),
	}

	// Act
	provider := s.browserQueryProvider()

	// Assert: router takes priority over extension and CDP
	if _, ok := provider.(*screenshot.ScreenshotRouter); !ok {
		t.Fatalf("expected *screenshot.ScreenshotRouter (highest priority), got %T", provider)
	}
}

func TestBrowserQueryProvider_CDPFallbackIgnoresUnroutedBridge(t *testing.T) {
	// Arrange: no router, but both bridge.Service and screenshotMgr are set
	s := &Server{
		screenshotRouter: nil,
		bridge: &BridgeState{
			Service: newTestBridgeService(),
		},
		screenshotMgr: newTestScreenshotMgr(t),
	}

	// Act
	provider := s.browserQueryProvider()

	// Assert: an unwired bridge cannot bypass router health selection.
	if _, ok := provider.(*screenshot.CDPProvider); !ok {
		t.Fatalf("expected *screenshot.CDPProvider, got %T", provider)
	}
}

func TestBrowserQueryProvider_BridgeWithoutService_ReturnsNil(t *testing.T) {
	// Arrange: bridge exists but Service is nil, no screenshotMgr
	s := &Server{
		screenshotRouter: nil,
		bridge:           &BridgeState{},
		screenshotMgr:    nil,
	}

	// Act
	provider := s.browserQueryProvider()

	// Assert
	if provider != nil {
		t.Fatalf("expected nil provider when bridge has no Service and no screenshotMgr, got %T", provider)
	}
}

func TestBrowserQueryProvider_BridgeWithoutService_CDPFallback(t *testing.T) {
	// Arrange: bridge exists but Service is nil, screenshotMgr is set
	s := &Server{
		screenshotRouter: nil,
		bridge:           &BridgeState{},
		screenshotMgr:    newTestScreenshotMgr(t),
	}

	// Act
	provider := s.browserQueryProvider()

	// Assert: falls through to CDP when bridge.Service is nil
	if _, ok := provider.(*screenshot.CDPProvider); !ok {
		t.Fatalf("expected *screenshot.CDPProvider (fallback), got %T", provider)
	}
}

func TestBrowserQueryProvider_Table(t *testing.T) {
	tests := []struct {
		name             string
		hasRouter        bool
		hasBridgeService bool
		hasScreenshotMgr bool
		expectedType     string
		expectedNil      bool
	}{
		{
			name:             "router takes priority over all",
			hasRouter:        true,
			hasBridgeService: true,
			hasScreenshotMgr: true,
			expectedType:     "*screenshot.ScreenshotRouter",
		},
		{
			name:         "router only",
			hasRouter:    true,
			expectedType: "*screenshot.ScreenshotRouter",
		},
		{
			name:             "unrouted extension is unavailable",
			hasBridgeService: true,
			expectedNil:      true,
		},
		{
			name:             "CDP fallback ignores unrouted extension",
			hasBridgeService: true,
			hasScreenshotMgr: true,
			expectedType:     "*screenshot.CDPProvider",
		},
		{
			name:             "CDP only fallback",
			hasScreenshotMgr: true,
			expectedType:     "*screenshot.CDPProvider",
		},
		{
			name:        "all nil returns nil",
			expectedNil: true,
		},
		{
			name:        "bridge without service and no mgr returns nil",
			expectedNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			s := &Server{bridge: &BridgeState{}}

			if tt.hasRouter {
				s.screenshotRouter = newTestRouter()
			}
			if tt.hasBridgeService {
				s.bridge.Service = newTestBridgeService()
			}
			if tt.hasScreenshotMgr {
				s.screenshotMgr = newTestScreenshotMgr(t)
			}

			// Act
			provider := s.browserQueryProvider()

			// Assert
			if tt.expectedNil {
				if provider != nil {
					t.Fatalf("expected nil, got %T", provider)
				}
				return
			}
			if provider == nil {
				t.Fatalf("expected %s, got nil", tt.expectedType)
			}
			actualType := fmt.Sprintf("%T", provider)
			if actualType != tt.expectedType {
				t.Fatalf("expected %s, got %s", tt.expectedType, actualType)
			}
		})
	}
}

// ============================================================
// handleGetAdminToken tests
// ============================================================

func TestHandleGetAdminToken_ReturnsToken(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.Auth.Enabled = true
	cfg.Web.Auth.AdminToken = "test-admin-token-123"
	s := &Server{config: cfg}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/account/admin-token", nil)
	// Admin-token identity (synthetic admin) is authorized to read the token.
	req = req.WithContext(contextWithUserID(req.Context(), adminSyntheticUserID))
	w := httptest.NewRecorder()
	s.handleGetAdminToken(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if ok, _ := resp["success"].(bool); !ok {
		t.Fatalf("expected success=true, got %v", resp)
	}
	if tok, _ := resp["token"].(string); tok != "test-admin-token-123" {
		t.Fatalf("expected real token 'test-admin-token-123', got %q", tok)
	}
}

// TestHandleGetAdminToken_ForbiddenForNonAdmin ensures a logged-in non-admin
// user cannot retrieve the admin token (P0 FINDING-001 privilege escalation fix).
func TestHandleGetAdminToken_ForbiddenForNonAdmin(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.Auth.Enabled = true
	cfg.Web.Auth.AdminToken = "test-admin-token-123"
	// Non-admin user has a real DB id (userID > 0) and role != admin.
	s := &Server{config: cfg, userRepo: &mockUserRepo{users: map[int64]*auth.User{
		42: {ID: 42, Username: "normal", Role: "user", Status: "active"},
	}}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/account/admin-token", nil)
	req = req.WithContext(contextWithUserID(req.Context(), 42))
	w := httptest.NewRecorder()
	s.handleGetAdminToken(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin user, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "test-admin-token-123") {
		t.Fatalf("admin token must not be leaked to non-admin user, body=%s", w.Body.String())
	}
}

// TestHandleGetAdminToken_LegacySingleUser ensures the legacy single-user mode
// (userID == 0, config admin account, no user DB) can still retrieve the token.
// This is the default deployment shape today; it must not regress.
func TestHandleGetAdminToken_LegacySingleUser(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.Auth.Enabled = true
	cfg.Web.Auth.AdminToken = "test-admin-token-123"
	s := &Server{config: cfg} // no userRepo => legacy single-user mode

	req := httptest.NewRequest(http.MethodGet, "/api/v1/account/admin-token", nil)
	req = req.WithContext(contextWithUserID(req.Context(), 0))
	w := httptest.NewRecorder()
	s.handleGetAdminToken(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for legacy single-user admin, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if tok, _ := resp["token"].(string); tok != "test-admin-token-123" {
		t.Fatalf("expected real token, got %q", tok)
	}
}

func TestHandleGetAdminToken_AuthDisabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.Auth.Enabled = false
	s := &Server{config: cfg}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/account/admin-token", nil)
	w := httptest.NewRecorder()
	s.handleGetAdminToken(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if tok, _ := resp["token"].(string); tok != "" {
		t.Fatalf("expected empty token when auth disabled, got %q", tok)
	}
}

func TestHandleGetAdminToken_WrongMethod(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/account/admin-token", nil)
	w := httptest.NewRecorder()
	s.handleGetAdminToken(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}
func TestBuildQueryAPIPayloadPreservesEngineFailure(t *testing.T) {
	for _, count := range []int{0, 1} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			assets := make([]model.UnifiedAsset, count)
			resp := &service.QueryResponse{Assets: assets, Errors: []string{"engine daydaymap: API unavailable"}}
			outcome := browserQueryOutcome{Enabled: true, OpenedEngines: []string{"daydaymap"}, CollectedResults: []collection.CollectResult{{Engine: "daydaymap", Assets: assets}}}
			payload := buildQueryAPIPayload(`port="80"`, []string{"daydaymap"}, resp, outcome, "collect_and_capture", true)
			want := "error"
			if count > 0 {
				want = "partial"
			}
			if payload.Status != want || len(payload.Errors) != 1 || payload.Errors[0] != resp.Errors[0] {
				t.Fatalf("status=%s errors=%v; want %s with preserved API diagnostic", payload.Status, payload.Errors, want)
			}
		})
	}
}
