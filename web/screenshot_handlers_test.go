package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/config"
	"github.com/unimap-icp-hunter/project/internal/screenshot"
	"github.com/unimap-icp-hunter/project/internal/service"
)

func buildTestServerWithScreenshotBase(baseDir string) *Server {
	cfg := &config.Config{}
	cfg.Screenshot.BaseDir = baseDir
	return &Server{
		config:        cfg,
		screenshotApp: service.NewScreenshotAppService(baseDir),
		screenshotMgr: &screenshot.Manager{},
	}
}

func TestNormalizeScreenshotPathToken(t *testing.T) {
	tests := []struct {
		in   string
		ok   bool
		want string
	}{
		{in: "batch-001", ok: true, want: "batch-001"},
		{in: "", ok: false},
		{in: "..", ok: false},
		{in: "a/b", ok: false},
		{in: "a\\b", ok: false},
	}

	for _, tt := range tests {
		got, ok := normalizeScreenshotPathToken(tt.in)
		if ok != tt.ok {
			t.Fatalf("input %q expected ok=%v got %v", tt.in, tt.ok, ok)
		}
		if ok && got != tt.want {
			t.Fatalf("input %q expected %q got %q", tt.in, tt.want, got)
		}
	}
}

func TestHandleScreenshotBatchesAndFiles(t *testing.T) {
	baseDir := t.TempDir()
	batchDir := filepath.Join(baseDir, "batch-a")
	if err := os.MkdirAll(batchDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(batchDir, "x.png"), []byte("x"), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	s := buildTestServerWithScreenshotBase(baseDir)

	req := httptest.NewRequest(http.MethodGet, "/api/screenshot/batches", nil)
	w := httptest.NewRecorder()
	s.handleScreenshotBatches(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var batchResp struct {
		Success bool `json:"success"`
		Count   int  `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &batchResp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if !batchResp.Success || batchResp.Count != 1 {
		t.Fatalf("unexpected batch response: %+v", batchResp)
	}

	reqFiles := httptest.NewRequest(http.MethodGet, "/api/screenshot/batches/files?batch=batch-a", nil)
	wFiles := httptest.NewRecorder()
	s.handleScreenshotBatchFiles(wFiles, reqFiles)
	if wFiles.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wFiles.Code)
	}

	var filesResp struct {
		Success bool `json:"success"`
		Count   int  `json:"count"`
	}
	if err := json.Unmarshal(wFiles.Body.Bytes(), &filesResp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if !filesResp.Success || filesResp.Count != 1 {
		t.Fatalf("unexpected files response: %+v", filesResp)
	}
}

func TestHandleScreenshotDeleteSafety(t *testing.T) {
	baseDir := t.TempDir()
	batchDir := filepath.Join(baseDir, "batch-z")
	if err := os.MkdirAll(batchDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(batchDir, "a.png"), []byte("x"), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	s := buildTestServerWithScreenshotBase(baseDir)

	badReq := httptest.NewRequest(http.MethodDelete, "/api/screenshot/file/delete?batch=../evil&file=a.png", nil)
	badW := httptest.NewRecorder()
	s.handleScreenshotFileDelete(badW, badReq)
	if badW.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for traversal, got %d", badW.Code)
	}

	okReq := httptest.NewRequest(http.MethodDelete, "/api/screenshot/file/delete?batch=batch-z&file=a.png", nil)
	okW := httptest.NewRecorder()
	s.handleScreenshotFileDelete(okW, okReq)
	if okW.Code != http.StatusOK {
		t.Fatalf("expected 200 for delete, got %d", okW.Code)
	}
	if _, err := os.Stat(filepath.Join(batchDir, "a.png")); !os.IsNotExist(err) {
		t.Fatalf("expected file deleted, stat err=%v", err)
	}
}

// ============================================================
// handleScreenshot error path tests
// ============================================================

func TestHandleScreenshot_WrongMethod(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/screenshot", nil)
	w := httptest.NewRecorder()
	s.handleScreenshot(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleScreenshot_MissingURL(t *testing.T) {
	s := &Server{}
	body := strings.NewReader(`{"url":""}`)
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot", body)
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://localhost:8448")
	w := httptest.NewRecorder()
	s.handleScreenshot(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "missing_url") {
		t.Fatalf("expected 'missing_url' in body, got %q", w.Body.String())
	}
}

func TestHandleScreenshot_EmptyBody(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot", nil)
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://localhost:8448")
	w := httptest.NewRecorder()
	s.handleScreenshot(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleScreenshot_PrivateIPBlocked(t *testing.T) {
	s := &Server{}
	body := strings.NewReader(`{"url":"http://127.0.0.1:8080"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot", body)
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://localhost:8448")
	w := httptest.NewRecorder()
	s.handleScreenshot(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "blocked_url") {
		t.Fatalf("expected 'blocked_url' in body, got %q", w.Body.String())
	}
}

func TestHandleScreenshot_PrivateIPBlockedLocalhost(t *testing.T) {
	s := &Server{}
	body := strings.NewReader(`{"url":"http://localhost:3000/admin"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot", body)
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://localhost:8448")
	w := httptest.NewRecorder()
	s.handleScreenshot(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "blocked_url") {
		t.Fatalf("expected 'blocked_url' in body, got %q", w.Body.String())
	}
}

// ============================================================
// handleSearchEngineScreenshot error path tests
// ============================================================

func TestHandleSearchEngineScreenshot_WrongMethod(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/engine", nil)
	w := httptest.NewRecorder()
	s.handleSearchEngineScreenshot(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleSearchEngineScreenshot_NoScreenshotApp(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/screenshot/engine?engine=fofa&query=test", nil)
	w := httptest.NewRecorder()
	s.handleSearchEngineScreenshot(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "screenshot_manager_unavailable") {
		t.Fatalf("expected 'screenshot_manager_unavailable' in body, got %q", w.Body.String())
	}
}

func TestHandleSearchEngineScreenshot_MissingQuery(t *testing.T) {
	s := buildTestServerWithScreenshotBase(t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/api/screenshot/engine?engine=fofa", nil)
	w := httptest.NewRecorder()
	s.handleSearchEngineScreenshot(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "missing_parameters") {
		t.Fatalf("expected 'missing_parameters' in body, got %q", w.Body.String())
	}
}

func TestHandleSearchEngineScreenshot_MissingEngine(t *testing.T) {
	s := buildTestServerWithScreenshotBase(t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/api/screenshot/engine?query=test", nil)
	w := httptest.NewRecorder()
	s.handleSearchEngineScreenshot(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "missing_parameters") {
		t.Fatalf("expected 'missing_parameters' in body, got %q", w.Body.String())
	}
}

// ============================================================
// handleBatchScreenshot error path tests
// ============================================================

func TestHandleBatchScreenshot_WrongMethod(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/screenshot/batch", nil)
	w := httptest.NewRecorder()
	s.handleBatchScreenshot(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleBatchScreenshot_NoScreenshotApp(t *testing.T) {
	s := &Server{}
	body := strings.NewReader(`{"query_id":"1","engines":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/batch", body)
	w := httptest.NewRecorder()
	s.handleBatchScreenshot(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "screenshot_manager_unavailable") {
		t.Fatalf("expected 'screenshot_manager_unavailable' in body, got %q", w.Body.String())
	}
}

func TestHandleBatchScreenshot_PrivateTargetURL(t *testing.T) {
	s := buildTestServerWithScreenshotBase(t.TempDir())
	body := strings.NewReader(`{"query_id":"1","targets":[{"url":"http://192.168.1.1:8080"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/batch", body)
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://localhost:8448")
	w := httptest.NewRecorder()
	s.handleBatchScreenshot(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "blocked_url") {
		t.Fatalf("expected 'blocked_url' in body, got %q", w.Body.String())
	}
}

func TestHandleBatchScreenshot_PrivateTargetIPField(t *testing.T) {
	s := buildTestServerWithScreenshotBase(t.TempDir())
	body := strings.NewReader(`{"query_id":"1","targets":[{"ip":"10.0.0.1"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/batch", body)
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://localhost:8448")
	w := httptest.NewRecorder()
	s.handleBatchScreenshot(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "blocked_url") {
		t.Fatalf("expected 'blocked_url' in body, got %q", w.Body.String())
	}
}

func TestHandleBatchScreenshot_InvalidJSON(t *testing.T) {
	s := buildTestServerWithScreenshotBase(t.TempDir())
	body := strings.NewReader(`not-json`)
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/batch", body)
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://localhost:8448")
	w := httptest.NewRecorder()
	s.handleBatchScreenshot(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ============================================================
// handleBatchURLsScreenshot error path tests
// ============================================================

func TestHandleBatchURLsScreenshot_WrongMethod(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/screenshot/batch-urls", nil)
	w := httptest.NewRecorder()
	s.handleBatchURLsScreenshot(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleBatchURLsScreenshot_NoScreenshotApp(t *testing.T) {
	s := &Server{}
	body := strings.NewReader(`{"urls":["https://example.com"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/batch-urls", body)
	w := httptest.NewRecorder()
	s.handleBatchURLsScreenshot(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "screenshot_manager_unavailable") {
		t.Fatalf("expected 'screenshot_manager_unavailable' in body, got %q", w.Body.String())
	}
}

func TestHandleBatchURLsScreenshot_PrivateIP(t *testing.T) {
	s := buildTestServerWithScreenshotBase(t.TempDir())
	body := strings.NewReader(`{"urls":["https://127.0.0.1:8080"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/batch-urls", body)
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://localhost:8448")
	w := httptest.NewRecorder()
	s.handleBatchURLsScreenshot(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "blocked_url") {
		t.Fatalf("expected 'blocked_url' in body, got %q", w.Body.String())
	}
}

func TestHandleBatchURLsScreenshot_InvalidURL(t *testing.T) {
	s := buildTestServerWithScreenshotBase(t.TempDir())
	body := strings.NewReader(`{"urls":["://invalid"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/batch-urls", body)
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://localhost:8448")
	w := httptest.NewRecorder()
	s.handleBatchURLsScreenshot(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid_url") {
		t.Fatalf("expected 'invalid_url' in body, got %q", w.Body.String())
	}
}

func TestHandleBatchURLsScreenshot_InvalidScheme(t *testing.T) {
	s := buildTestServerWithScreenshotBase(t.TempDir())
	body := strings.NewReader(`{"urls":["file:///etc/passwd"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/batch-urls", body)
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://localhost:8448")
	w := httptest.NewRecorder()
	s.handleBatchURLsScreenshot(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid_url_scheme") {
		t.Fatalf("expected 'invalid_url_scheme' in body, got %q", w.Body.String())
	}
}

func TestHandleBatchURLsScreenshot_InvalidJSON(t *testing.T) {
	s := buildTestServerWithScreenshotBase(t.TempDir())
	body := strings.NewReader(`not-json`)
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/batch-urls", body)
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://localhost:8448")
	w := httptest.NewRecorder()
	s.handleBatchURLsScreenshot(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ============================================================
// handleScreenshotRouterStatus tests
// ============================================================

func TestHandleScreenshotRouterStatus(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/screenshot/router/status", nil)
	w := httptest.NewRecorder()
	s.handleScreenshotRouterStatus(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 200 or 503, got %d", w.Code)
	}
}

// ============================================================
// resolveScreenshotBatchDir tests
// ============================================================

func TestResolveScreenshotBatchDir_Default(t *testing.T) {
	s := &Server{}
	got, ok := s.resolveScreenshotBatchDir("test-batch")
	if !ok {
		t.Fatal("expected ok=true for valid batch token")
	}
	if got == "" {
		t.Fatal("expected non-empty batch dir")
	}
	if !strings.Contains(got, "screenshots") {
		t.Fatalf("expected path containing 'screenshots', got %q", got)
	}
}

func TestResolveScreenshotBatchDir_InvalidBatch(t *testing.T) {
	s := &Server{}
	_, ok := s.resolveScreenshotBatchDir("")
	if ok {
		t.Fatal("expected ok=false for empty batch token")
	}
}

func TestResolveScreenshotBatchDir_TraversalBatch(t *testing.T) {
	s := &Server{}
	_, ok := s.resolveScreenshotBatchDir("../evil")
	if ok {
		t.Fatal("expected ok=false for traversal batch token")
	}
}

// ============================================================
// handleScreenshotBatchDelete tests
// ============================================================

func TestHandleScreenshotBatchDelete_WrongMethod(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/screenshot/batch/test", nil)
	w := httptest.NewRecorder()
	s.handleScreenshotBatchDelete(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// ============================================================
// handleScreenshotFileDelete tests
// ============================================================

func TestHandleScreenshotFileDelete_WrongMethod(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/screenshot/file/test.png", nil)
	w := httptest.NewRecorder()
	s.handleScreenshotFileDelete(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

