package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ============================================================
// http_helpers: isOriginAllowed table-driven tests
// ============================================================

func TestIsOriginAllowed_EmptyOrigin(t *testing.T) {
	if !isOriginAllowed("", "example.com", []string{"https://other.com"}) {
		t.Fatal("expected empty origin to be allowed")
	}
}

func TestIsOriginAllowed_SameHost(t *testing.T) {
	if !isOriginAllowed("http://example.com/page", "example.com", nil) {
		t.Fatal("expected same host to be allowed")
	}
}

func TestIsOriginAllowed_ListMatch(t *testing.T) {
	if !isOriginAllowed("https://allowed.com", "other.com", []string{"https://allowed.com"}) {
		t.Fatal("expected list match to be allowed")
	}
}

func TestIsOriginAllowed_Rejected(t *testing.T) {
	if isOriginAllowed("https://evil.com", "example.com", []string{"https://allowed.com"}) {
		t.Fatal("expected evil origin to be rejected")
	}
}

// ============================================================
// http_helpers: requireTrustedRequest tests
// ============================================================

func TestRequireTrustedRequest_Trusted(t *testing.T) {
	// GET requests without Origin/Referer are trusted
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	if !requireTrustedRequest(w, req, []string{"https://trusted.com"}) {
		t.Fatal("expected requireTrustedRequest to return true for GET request (no origin)")
	}
}

func TestRequireTrustedRequest_Untrusted(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Origin", "https://evil.com")
	req.Header.Set("Referer", "https://evil.com/page")
	w := httptest.NewRecorder()
	if requireTrustedRequest(w, req, []string{"https://trusted.com"}) {
		t.Fatal("expected requireTrustedRequest to return false for untrusted request")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "forbidden_origin") {
		t.Fatalf("expected error body to contain 'forbidden_origin', got %q", w.Body.String())
	}
}

// ============================================================
// http_helpers: decodeJSONBody extra coverage
// ============================================================

func TestDecodeJSONBody_NilBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	var result struct{ Name string }
	if decodeJSONBody(w, req, &result) {
		t.Fatal("expected decodeJSONBody to return false for nil body")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ============================================================
// http_helpers: writeAPIError with nil details
// ============================================================

func TestWriteAPIError_NilDetails(t *testing.T) {
	w := httptest.NewRecorder()
	writeAPIError(w, http.StatusNotFound, "not_found", "resource not found", nil)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"success":false`) {
		t.Fatalf("expected success:false in body, got %q", body)
	}
	if !strings.Contains(body, `"code":"not_found"`) {
		t.Fatalf("expected code in body, got %q", body)
	}
}

func TestWriteAPIError_StringDetails(t *testing.T) {
	w := httptest.NewRecorder()
	writeAPIError(w, http.StatusInternalServerError, "internal", "error", "detail string")

	body := w.Body.String()
	if !strings.Contains(body, `"detail string"`) {
		t.Fatalf("expected details in body, got %q", body)
	}
}

// ============================================================
// http_helpers: requestSizeLimitMiddleware extra coverage
// ============================================================

func TestRequestSizeLimitMiddleware_DefaultMaxBody(t *testing.T) {
	handler := requestSizeLimitMiddleware(-1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with default max body, got %d", w.Code)
	}
}

func TestRequestSizeLimitMiddleware_WebSocketBypass(t *testing.T) {
	handler := requestSizeLimitMiddleware(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"data":"large payload that exceeds limit"}`))
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for WebSocket bypass, got %d", w.Code)
	}
}

func TestRequestSizeLimitMiddleware_PutMethod(t *testing.T) {
	handler := requestSizeLimitMiddleware(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next should not be called")
	}))

	body := bytes.NewBufferString(`{"this body is way too large for the limit"}`)
	req := httptest.NewRequest(http.MethodPut, "/", body)
	req.ContentLength = int64(body.Len())
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for PUT, got %d", w.Code)
	}
}

func TestRequestSizeLimitMiddleware_DeleteMethod(t *testing.T) {
	handler := requestSizeLimitMiddleware(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next should not be called")
	}))

	body := bytes.NewBufferString(`{"this body is way too large"}`)
	req := httptest.NewRequest(http.MethodDelete, "/", body)
	req.ContentLength = int64(body.Len())
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for DELETE, got %d", w.Code)
	}
}

func TestRequestSizeLimitMiddleware_PatchMethod(t *testing.T) {
	handler := requestSizeLimitMiddleware(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next should not be called")
	}))

	body := bytes.NewBufferString(`{"too large for patch"}`)
	req := httptest.NewRequest(http.MethodPatch, "/", body)
	req.ContentLength = int64(body.Len())
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for PATCH, got %d", w.Code)
	}
}

// ============================================================
// http_helpers: corsMiddleware extra coverage
// ============================================================

func TestCORSMiddleware_ExposeHeaders(t *testing.T) {
	handler := corsMiddleware([]string{"https://example.com"}, nil, nil, []string{"X-Request-Id", "X-Custom"}, true, 0)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	exposed := w.Header().Get("Access-Control-Expose-Headers")
	if exposed != "X-Request-Id, X-Custom" {
		t.Fatalf("expected expose headers 'X-Request-Id, X-Custom', got %q", exposed)
	}
}

func TestCORSMiddleware_MaxAgeZero(t *testing.T) {
	handler := corsMiddleware([]string{"https://example.com"}, nil, nil, nil, true, 0)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Max-Age") != "" {
		t.Fatalf("expected no Max-Age header when maxAge=0, got %q", w.Header().Get("Access-Control-Max-Age"))
	}
}

func TestCORSMiddleware_MaxAgeNegative(t *testing.T) {
	handler := corsMiddleware(nil, nil, nil, nil, false, -100)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// negative maxAge gets clamped to 0, so no Max-Age header
	if w.Header().Get("Access-Control-Max-Age") != "" {
		t.Fatalf("expected no Max-Age header for negative maxAge, got %q", w.Header().Get("Access-Control-Max-Age"))
	}
}

func TestCORSMiddleware_NoCredentials(t *testing.T) {
	handler := corsMiddleware([]string{"https://example.com"}, nil, nil, nil, false, 0)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("expected no credentials header when allowCredentials=false")
	}
}

// ============================================================
// render.go: template rendering tests
// ============================================================

func TestRenderTemplate_NilServer(t *testing.T) {
	var s *Server
	w := httptest.NewRecorder()
	result := s.renderTemplate(w, http.StatusInternalServerError, "index.html", nil)

	if result {
		t.Fatal("expected false for nil server")
	}
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestRenderTemplate_NilTemplates(t *testing.T) {
	s := &Server{templates: nil}
	w := httptest.NewRecorder()
	result := s.renderTemplate(w, http.StatusInternalServerError, "index.html", nil)

	if result {
		t.Fatal("expected false for nil templates")
	}
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// ============================================================
// securityMiddleware tests
// ============================================================

func TestSecurityMiddleware_Headers(t *testing.T) {
	handler := securityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	tests := []struct {
		header string
		want   string
	}{
		{"X-Frame-Options", "DENY"},
		{"X-Content-Type-Options", "nosniff"},
		{"X-XSS-Protection", "1; mode=block"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
		{"Permissions-Policy", "geolocation=(), microphone=(), camera=()"},
	}
	for _, tt := range tests {
		got := w.Header().Get(tt.header)
		if got != tt.want {
			t.Errorf("header %q = %q, want %q", tt.header, got, tt.want)
		}
	}
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP missing or incorrect: %q", csp)
	}
}

func TestSecurityMiddleware_CallsNext(t *testing.T) {
	called := false
	handler := securityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Fatal("expected next handler to be called")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
