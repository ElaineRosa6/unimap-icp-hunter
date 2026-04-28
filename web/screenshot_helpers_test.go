package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/config"
)

// ============================================================
// resolveScreenshotBaseDir tests
// ============================================================

func TestResolveScreenshotBaseDir_Default(t *testing.T) {
	s := &Server{}
	got := s.resolveScreenshotBaseDir()
	if !strings.HasSuffix(filepath.ToSlash(got), "screenshots") {
		t.Fatalf("expected path ending in 'screenshots', got %q", got)
	}
}

func TestResolveScreenshotBaseDir_FromConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Screenshot.BaseDir = "/tmp/test-screenshots"
	s := &Server{config: cfg}
	got := s.resolveScreenshotBaseDir()
	// On Windows, /tmp/... becomes D:\tmp\...; check the path suffix instead
	if !strings.HasSuffix(filepath.ToSlash(got), "tmp/test-screenshots") {
		t.Fatalf("expected path ending in 'tmp/test-screenshots', got %q", got)
	}
}

func TestResolveScreenshotBaseDir_RelativePath(t *testing.T) {
	cfg := &config.Config{}
	cfg.Screenshot.BaseDir = "./data/screenshots"
	s := &Server{config: cfg}
	got := s.resolveScreenshotBaseDir()
	if !filepath.IsAbs(got) {
		t.Fatalf("expected absolute path, got %q", got)
	}
	if !strings.HasSuffix(filepath.ToSlash(got), "data/screenshots") {
		t.Fatalf("expected path ending in 'data/screenshots', got %q", got)
	}
}

// ============================================================
// screenshotPathToPreviewURL tests
// ============================================================

func TestScreenshotPathToPreviewURL_Empty(t *testing.T) {
	s := &Server{}
	got := s.screenshotPathToPreviewURL("")
	if got != "" {
		t.Fatalf("expected empty string for empty path, got %q", got)
	}
}

func TestScreenshotPathToPreviewURL_Relative(t *testing.T) {
	cfg := &config.Config{}
	cfg.Screenshot.BaseDir = "/tmp/screenshots"
	s := &Server{config: cfg}

	got := s.screenshotPathToPreviewURL("/tmp/screenshots/batch1/test.png")
	if got != "/screenshots/batch1/test.png" {
		t.Fatalf("expected '/screenshots/batch1/test.png', got %q", got)
	}
}

func TestScreenshotPathToPreviewURL_OutsideBase(t *testing.T) {
	cfg := &config.Config{}
	cfg.Screenshot.BaseDir = "/tmp/screenshots"
	s := &Server{config: cfg}

	got := s.screenshotPathToPreviewURL("/etc/passwd")
	if got != "" {
		t.Fatalf("expected empty string for path outside base, got %q", got)
	}
}

// ============================================================
// handleScreenshotFile tests
// ============================================================

func TestHandleScreenshotFile_WrongMethod(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/screenshots/test.png", nil)
	w := httptest.NewRecorder()
	s.handleScreenshotFile(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleScreenshotFile_ForbiddenOrigin(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.CORS.AllowedOrigins = []string{"https://allowed.com"}
	s := &Server{config: cfg}

	req := httptest.NewRequest(http.MethodGet, "/screenshots/bad-origin.png", nil)
	req.Header.Set("Origin", "https://evil.com")
	req.Header.Set("Referer", "https://evil.com/page")
	w := httptest.NewRecorder()
	s.handleScreenshotFile(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "forbidden_origin") {
		t.Fatalf("expected 'forbidden_origin' in body, got %q", w.Body.String())
	}
}

func TestHandleScreenshotFile_PathTraversal(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/screenshots/../../etc/passwd", nil)
	w := httptest.NewRecorder()
	s.handleScreenshotFile(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for path traversal, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid_path") {
		t.Fatalf("expected 'invalid_path' in body, got %q", w.Body.String())
	}
}

func TestHandleScreenshotFile_UnsupportedFileType(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/screenshots/test.gif", nil)
	w := httptest.NewRecorder()
	s.handleScreenshotFile(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for unsupported file type, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "unsupported_file_type") {
		t.Fatalf("expected 'unsupported_file_type' in body, got %q", w.Body.String())
	}
}

func TestHandleScreenshotFile_NotFound(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/screenshots/nonexistent.png", nil)
	w := httptest.NewRecorder()
	s.handleScreenshotFile(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleScreenshotFile_TrailingSlash(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/screenshots/", nil)
	w := httptest.NewRecorder()
	s.handleScreenshotFile(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for trailing slash, got %d", w.Code)
	}
}

func TestHandleScreenshotFile_AcceptedImageTypes(t *testing.T) {
	s := &Server{}
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp"} {
		req := httptest.NewRequest(http.MethodGet, "/screenshots/test"+ext, nil)
		w := httptest.NewRecorder()
		s.handleScreenshotFile(w, req)

		// Should pass file type check and return 404 (file doesn't exist)
		// Not 403 (forbidden file type)
		if w.Code == http.StatusForbidden && strings.Contains(w.Body.String(), "unsupported_file_type") {
			t.Errorf("expected %s to pass file type check, got 403 forbidden", ext)
		}
	}
}

// ============================================================
// isPrivateOrInternalIP tests
// ============================================================

func TestIsPrivateOrInternalIP_Localhost(t *testing.T) {
	tests := []string{"localhost", "127.0.0.1", "::1", "0.0.0.0"}
	for _, host := range tests {
		if !isPrivateOrInternalIP(host) {
			t.Errorf("expected %q to be private/internal", host)
		}
	}
}

func TestIsPrivateOrInternalIP_PublicIP(t *testing.T) {
	if isPrivateOrInternalIP("8.8.8.8") {
		t.Fatal("expected 8.8.8.8 to NOT be private/internal")
	}
	if isPrivateOrInternalIP("1.1.1.1") {
		t.Fatal("expected 1.1.1.1 to NOT be private/internal")
	}
}

func TestIsPrivateOrInternalIP_PrivateIP(t *testing.T) {
	tests := []string{"192.168.1.1", "10.0.0.1", "172.16.0.1"}
	for _, host := range tests {
		if !isPrivateOrInternalIP(host) {
			t.Errorf("expected %q to be private/internal", host)
		}
	}
}

func TestIsPrivateOrInternalIP_InvalidIP(t *testing.T) {
	if isPrivateOrInternalIP("not-an-ip") {
		t.Fatal("expected 'not-an-ip' to NOT be private/internal")
	}
}

func TestIsPrivateOrInternalIP_Hostname(t *testing.T) {
	if isPrivateOrInternalIP("example.com") {
		t.Fatal("expected hostname to NOT be private/internal")
	}
}

// ============================================================
// allowedOriginsFromConfig tests
// ============================================================

func TestAllowedOriginsFromConfig_NilConfig(t *testing.T) {
	got := allowedOriginsFromConfig(nil)
	if len(got) != 2 {
		t.Fatalf("expected 2 default origins, got %d", len(got))
	}
	if got[0] != "http://localhost:8448" {
		t.Fatalf("expected first origin http://localhost:8448, got %q", got[0])
	}
}

func TestAllowedOriginsFromConfig_CustomPort(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.Port = 9000
	got := allowedOriginsFromConfig(cfg)
	if len(got) != 2 {
		t.Fatalf("expected 2 default origins, got %d", len(got))
	}
	if got[0] != "http://localhost:9000" {
		t.Fatalf("expected first origin http://localhost:9000, got %q", got[0])
	}
}

func TestAllowedOriginsFromConfig_ExplicitOrigins(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.CORS.AllowedOrigins = []string{"https://myapp.com", "http://localhost:3000"}
	got := allowedOriginsFromConfig(cfg)
	if len(got) != 2 {
		t.Fatalf("expected 2 origins, got %d", len(got))
	}
	if got[0] != "https://myapp.com" {
		t.Fatalf("expected first origin https://myapp.com, got %q", got[0])
	}
}

func TestAllowedOriginsFromConfig_TrimsWhitespace(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.CORS.AllowedOrigins = []string{"  https://myapp.com  ", "", "  "}
	got := allowedOriginsFromConfig(cfg)
	if len(got) != 1 {
		t.Fatalf("expected 1 origin after trimming, got %d", len(got))
	}
	if got[0] != "https://myapp.com" {
		t.Fatalf("expected 'https://myapp.com', got %q", got[0])
	}
}
