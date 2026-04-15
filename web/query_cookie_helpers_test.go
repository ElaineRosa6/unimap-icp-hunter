package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/config"
	"github.com/unimap-icp-hunter/project/internal/distributed"
)

// ============================================================
// Query handler tests
// ============================================================

func TestHandleQueryStatus_MissingQueryID(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/query/status", nil)
	w := httptest.NewRecorder()
	s.handleQueryStatus(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleQueryStatus_NotFound(t *testing.T) {
	s := &Server{
		queryStatus: make(map[string]*QueryStatus),
	}
	s.queryStatus["q1"] = &QueryStatus{Status: "running"}

	req := httptest.NewRequest(http.MethodGet, "/api/query/status?query_id=nonexistent", nil)
	w := httptest.NewRecorder()
	s.handleQueryStatus(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleQueryStatus_Found(t *testing.T) {
	s := &Server{
		queryStatus: make(map[string]*QueryStatus),
	}
	s.queryStatus["q1"] = &QueryStatus{Status: "completed", TotalCount: 10}

	req := httptest.NewRequest(http.MethodGet, "/api/query/status?query_id=q1", nil)
	w := httptest.NewRecorder()
	s.handleQueryStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp QueryStatus
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.Status != "completed" || resp.TotalCount != 10 {
		t.Fatalf("unexpected status: %+v", resp)
	}
}

// ============================================================
// Health handler tests
// ============================================================

func TestHandleHealth(t *testing.T) {
	s := &Server{
		orchestrator: adapter.NewEngineOrchestrator(),
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected status 'ok', got %q", resp.Status)
	}
}

// ============================================================
// Cookie handler tests
// ============================================================

func TestHandleImportCookieJSON_MissingParams(t *testing.T) {
	cfg := &config.Config{}
	cfg.Distributed.Enabled = true
	s := &Server{
		config:      cfg,
		distributed: &DistributedState{NodeRegistry: distributed.NewRegistry(60 * time.Second)},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/cookies/import/fofa", nil)
	w := httptest.NewRecorder()
	s.handleImportCookieJSON(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing params, got %d", w.Code)
	}
}

func TestHandleImportCookieJSON_InvalidEngine(t *testing.T) {
	cfg := &config.Config{}
	cfg.Distributed.Enabled = true
	s := &Server{
		config:      cfg,
		distributed: &DistributedState{NodeRegistry: distributed.NewRegistry(60 * time.Second)},
	}

	form := "engine=unknown&cookie_json=%5B%7B%22name%22%3A%22token%22%2C%22value%22%3A%22abc%22%7D%5D"
	req := httptest.NewRequest(http.MethodPost, "/api/cookies/import/unknown", bytes.NewBufferString(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.handleImportCookieJSON(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported engine, got %d", w.Code)
	}
}

func TestHandleImportCookieJSON_Success(t *testing.T) {
	cfg := &config.Config{}
	cfg.Distributed.Enabled = true
	s := &Server{
		config:      cfg,
		distributed: &DistributedState{NodeRegistry: distributed.NewRegistry(60 * time.Second)},
	}

	// Valid cookie JSON for fofa
	form := "engine=fofa&cookie_json=%5B%7B%22name%22%3A%22token%22%2C%22value%22%3A%22abc123%22%2C%22domain%22%3A%22.fofa.info%22%7D%5D"
	req := httptest.NewRequest(http.MethodPost, "/api/cookies/import/fofa", bytes.NewBufferString(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.handleImportCookieJSON(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp["success"] != true {
		t.Fatalf("expected success=true, got %+v", resp)
	}
}

func TestHandleImportCookieJSON_ExtensionMode(t *testing.T) {
	cfg := &config.Config{}
	cfg.Distributed.Enabled = true
	cfg.Screenshot.Engine = "extension"
	s := &Server{
		config:      cfg,
		distributed: &DistributedState{NodeRegistry: distributed.NewRegistry(60 * time.Second)},
	}

	form := "engine=fofa&cookie_json=%5B%7B%22name%22%3A%22token%22%2C%22value%22%3A%22abc123%22%2C%22domain%22%3A%22.fofa.info%22%7D%5D"
	req := httptest.NewRequest(http.MethodPost, "/api/cookies/import/fofa", bytes.NewBufferString(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.handleImportCookieJSON(w, req)

	// Extension mode should succeed with a message about optional cookies
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for extension mode, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestHandleSaveCookies_CDPMode(t *testing.T) {
	cfg := &config.Config{}
	cfg.Distributed.Enabled = true
	s := &Server{
		config:      cfg,
		distributed: &DistributedState{NodeRegistry: distributed.NewRegistry(60 * time.Second)},
	}

	form := "cookie_fofa=token=abc"
	req := httptest.NewRequest(http.MethodPost, "/api/cookies", bytes.NewBufferString(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.handleSaveCookies(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestHandleSaveCookies_ExtensionMode(t *testing.T) {
	cfg := &config.Config{}
	cfg.Distributed.Enabled = true
	cfg.Screenshot.Engine = "extension"
	s := &Server{
		config:      cfg,
		distributed: &DistributedState{NodeRegistry: distributed.NewRegistry(60 * time.Second)},
	}

	form := "cookie_fofa=token=abc"
	req := httptest.NewRequest(http.MethodPost, "/api/cookies", bytes.NewBufferString(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.handleSaveCookies(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp["engine"] != "extension" {
		t.Fatalf("expected engine=extension, got %v", resp["engine"])
	}
}

// ============================================================
// HTTP helpers tests
// ============================================================

func TestRequireMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	if requireMethod(w, req, http.MethodPost) {
		t.Fatal("expected false for wrong method")
	}
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/test", nil)
	w2 := httptest.NewRecorder()

	if !requireMethod(w2, req2, http.MethodPost) {
		t.Fatal("expected true for correct method")
	}
}

func TestDecodeJSONBody_Empty(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(""))
	w := httptest.NewRecorder()

	var dst struct{}
	if decodeJSONBody(w, req, &dst) {
		t.Fatal("expected false for empty body")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDecodeJSONBody_Valid(t *testing.T) {
	body := `{"name":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	var dst struct {
		Name string `json:"name"`
	}
	if !decodeJSONBody(w, req, &dst) {
		t.Fatal("expected true for valid JSON")
	}
	if dst.Name != "test" {
		t.Fatalf("expected 'test', got %q", dst.Name)
	}
}

func TestDecodeJSONBody_TrailingData(t *testing.T) {
	body := `{"name":"test"}{"extra":1}`
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	var dst struct {
		Name string `json:"name"`
	}
	if decodeJSONBody(w, req, &dst) {
		t.Fatal("expected false for trailing data")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDecodeJSONBody_UnknownFields(t *testing.T) {
	body := `{"unknown":"value"}`
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	var dst struct {
		Name string `json:"name"`
	}
	if decodeJSONBody(w, req, &dst) {
		t.Fatal("expected false for unknown fields")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ============================================================
// Origin/trust tests
// ============================================================

func TestIsTrustedRequest_NoOriginNoReferer(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	if !isTrustedRequest(req, nil) {
		t.Fatal("expected trusted when no origin/referer")
	}
}

func TestIsTrustedRequest_SameHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Host = "example.com"
	req.Header.Set("Origin", "http://example.com")

	if !isTrustedRequest(req, nil) {
		t.Fatal("expected trusted for same host")
	}
}

func TestIsTrustedRequest_WildcardAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Host = "example.com"
	req.Header.Set("Origin", "http://other.com")

	if !isTrustedRequest(req, []string{"*"}) {
		t.Fatal("expected trusted for wildcard")
	}
}

func TestIsTrustedRequest_DisallowedOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Host = "example.com"
	req.Header.Set("Origin", "http://evil.com")
	req.Header.Set("Referer", "http://evil.com/other")

	if isTrustedRequest(req, []string{"http://allowed.com"}) {
		t.Fatal("expected not trusted for disallowed origin")
	}
}

func TestNormalizeOrigin_Valid(t *testing.T) {
	got := normalizeOrigin("http://example.com/path?query=1")
	if got != "http://example.com" {
		t.Fatalf("expected 'http://example.com', got %q", got)
	}
}

func TestNormalizeOrigin_Invalid(t *testing.T) {
	got := normalizeOrigin("not-a-url")
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestOriginAllowedByList_Star(t *testing.T) {
	if !originAllowedByList("http://evil.com", []string{"*"}) {
		t.Fatal("expected star to allow any")
	}
}

func TestOriginAllowedByList_Exact(t *testing.T) {
	if !originAllowedByList("http://allowed.com", []string{"http://allowed.com"}) {
		t.Fatal("expected exact match")
	}
	if originAllowedByList("http://other.com", []string{"http://allowed.com"}) {
		t.Fatal("expected not allowed")
	}
}

func TestIsSameHostURL(t *testing.T) {
	if !isSameHostURL("http://example.com", "example.com") {
		t.Fatal("expected same host")
	}
	if isSameHostURL("http://other.com", "example.com") {
		t.Fatal("expected different host")
	}
	if isSameHostURL("", "example.com") {
		t.Fatal("expected empty to fail")
	}
}

// ============================================================
// CORS middleware tests
// ============================================================

func TestCORSMiddleware_PreflightAllowed(t *testing.T) {
	handler := corsMiddleware([]string{"*"}, nil, nil, nil, false, 0)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()

	handler(next).ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for preflight, got %d", w.Code)
	}
}

func TestCORSMiddleware_PreflightDenied(t *testing.T) {
	handler := corsMiddleware([]string{"http://allowed.com"}, nil, nil, nil, false, 0)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "http://evil.com")
	w := httptest.NewRecorder()

	handler(next).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disallowed preflight, got %d", w.Code)
	}
}

func TestCORSMiddleware_NormalRequest(t *testing.T) {
	handler := corsMiddleware([]string{"http://allowed.com"}, nil, nil, nil, true, 3600)
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://allowed.com")
	w := httptest.NewRecorder()

	handler(next).ServeHTTP(w, req)

	if !nextCalled {
		t.Fatal("expected next handler to be called")
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "http://allowed.com" {
		t.Fatalf("expected CORS header, got %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("expected credentials header")
	}
}

// ============================================================
// Request size limit middleware tests
// ============================================================

func TestRequestSizeLimitMiddleware_WithinLimit(t *testing.T) {
	handler := requestSizeLimitMiddleware(1024)
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	body := bytes.Repeat([]byte("x"), 100)
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	w := httptest.NewRecorder()

	handler(next).ServeHTTP(w, req)

	if !nextCalled {
		t.Fatal("expected next handler to be called")
	}
}

func TestRequestSizeLimitMiddleware_ExceedsLimit(t *testing.T) {
	handler := requestSizeLimitMiddleware(100)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	})

	body := bytes.Repeat([]byte("x"), 200)
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	w := httptest.NewRecorder()

	handler(next).ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", w.Code)
	}
}

func TestRequestSizeLimitMiddleware_GetIgnored(t *testing.T) {
	handler := requestSizeLimitMiddleware(100)
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.ContentLength = 200
	w := httptest.NewRecorder()

	handler(next).ServeHTTP(w, req)

	if !nextCalled {
		t.Fatal("GET requests should bypass size limit")
	}
}

// ============================================================
// WriteJSON helper tests
// ============================================================

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusCreated, map[string]string{"key": "value"})

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp["key"] != "value" {
		t.Fatalf("expected 'value', got %q", resp["key"])
	}
}

func TestWriteAPIError(t *testing.T) {
	w := httptest.NewRecorder()
	writeAPIError(w, http.StatusBadRequest, "test_code", "test message", map[string]string{"detail": "extra"})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var resp struct {
		Success bool `json:"success"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.Success != false || resp.Error.Code != "test_code" || resp.Error.Message != "test message" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}
