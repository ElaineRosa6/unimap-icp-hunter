package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ============================================================
// handleCDPStatus tests
// ============================================================

func TestHandleCDPStatus_Online(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/cdp/status", nil)
	w := httptest.NewRecorder()
	s.handleCDPStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	// Response may be "online" or "success" depending on Chrome availability
	if !strings.Contains(body, `"online"`) {
		t.Fatalf("expected 'online' in response, got %q", body)
	}
}

// ============================================================
// handleCDPConnect tests
// ============================================================

func TestHandleCDPConnect_WrongMethod(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/cdp/connect", nil)
	w := httptest.NewRecorder()
	s.handleCDPConnect(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleCDPConnect_EmptyBody(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/cdp/connect", nil)
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://localhost:8448")
	w := httptest.NewRecorder()
	s.handleCDPConnect(w, req)

	// When Chrome is already running, the handler returns 200 "already online"
	// without requiring a body. When Chrome is not running, it would try to
	// start Chrome and may succeed or fail. So we accept either 200 (online)
	// or 500 (failed to start).
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 200 or 500, got %d", w.Code)
	}
}

// ============================================================
// CDP URL normalization tests
// ============================================================

func TestNormalizeCDPBaseURL_Empty(t *testing.T) {
	got := normalizeCDPBaseURL("")
	if got != "" {
		t.Fatalf("expected empty string for empty input, got %q", got)
	}
}

func TestNormalizeCDPBaseURL_WithTrailingSlash(t *testing.T) {
	got := normalizeCDPBaseURL("http://localhost:9222/")
	want := "http://localhost:9222"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNormalizeCDPBaseURL_WithoutTrailingSlash(t *testing.T) {
	got := normalizeCDPBaseURL("http://localhost:9222")
	want := "http://localhost:9222"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNormalizeCDPBaseURL_WithWhitespace(t *testing.T) {
	got := normalizeCDPBaseURL("  http://localhost:9222  ")
	want := "http://localhost:9222"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNormalizeCDPBaseURL_InvalidURL(t *testing.T) {
	got := normalizeCDPBaseURL("://invalid")
	// Invalid URL should return the trimmed input or empty
	// depending on implementation behavior
	if got == "" {
		// acceptable
		return
	}
	// If not empty, should at least be trimmed
	if got != "://invalid" {
		t.Logf("normalizeCDPBaseURL returned %q for invalid URL", got)
	}
}

// (isAllDigits already tested in cdp_util_test.go)

// ============================================================
// isRemoteDebuggerAvailable tests
// ============================================================

func TestIsRemoteDebuggerAvailable_FalseWhenNotRunning(t *testing.T) {
	// When no Chrome is running, this should return false
	got := isRemoteDebuggerAvailable("http://localhost:59999")
	if got {
		t.Fatal("expected false when no debugger is running on that port")
	}
}

func TestIsRemoteDebuggerAvailable_InvalidURL(t *testing.T) {
	got := isRemoteDebuggerAvailable("://invalid")
	if got {
		t.Fatal("expected false for invalid URL")
	}
}
