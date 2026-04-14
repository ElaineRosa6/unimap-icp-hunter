package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/config"
)

func TestAdminAuthMiddleware_MissingToken_Returns401(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Web.Auth.Enabled = true
	s.config.Web.Auth.AdminToken = "secret-token"

	mw := s.adminAuthMiddleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAdminAuthMiddleware_WrongToken_Returns401(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Web.Auth.Enabled = true
	s.config.Web.Auth.AdminToken = "secret-token"

	mw := s.adminAuthMiddleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.Header.Set("X-Admin-Token", "wrong-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAdminAuthMiddleware_ValidHeader_Passes(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Web.Auth.Enabled = true
	s.config.Web.Auth.AdminToken = "secret-token"

	mw := s.adminAuthMiddleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.Header.Set("X-Admin-Token", "secret-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAdminAuthMiddleware_ValidQuery_Passes(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Web.Auth.Enabled = true
	s.config.Web.Auth.AdminToken = "secret-token"

	mw := s.adminAuthMiddleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard?admin_token=secret-token", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAdminAuthMiddleware_PublicPath_SkipsAuth(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Web.Auth.Enabled = true
	s.config.Web.Auth.AdminToken = "secret-token"

	mw := s.adminAuthMiddleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	paths := []string{"/health", "/health/live", "/static/css/style.css", "/screenshots/img.png"}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("path %s: expected 200, got %d", path, rec.Code)
		}
	}
}

func TestAdminAuthMiddleware_AuthDisabled_Passes(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Web.Auth.Enabled = false

	mw := s.adminAuthMiddleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Auth disabled but no token configured — should still reject non-public paths
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when auth disabled but no token configured, got %d", rec.Code)
	}
}

func TestIsPublicPath(t *testing.T) {
	s := &Server{}

	tests := []struct {
		path     string
		expected bool
	}{
		{"/health", true},
		{"/health/live", true},
		{"/static/js/main.js", true},
		{"/screenshots/test.png", true},
		{"/dashboard", false},
		{"/api/query", false},
		{"/api/nodes/status", false},
	}

	for _, tc := range tests {
		if got := s.isPublicPath(tc.path); got != tc.expected {
			t.Errorf("isPublicPath(%q) = %v, want %v", tc.path, got, tc.expected)
		}
	}
}

func TestAdminToken(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Web.Auth.Enabled = true
	s.config.Web.Auth.AdminToken = "my-token"

	if got := s.adminToken(); got != "my-token" {
		t.Fatalf("expected 'my-token', got %q", got)
	}

	s.config.Web.Auth.Enabled = false
	if got := s.adminToken(); got != "" {
		t.Fatalf("expected empty when auth disabled, got %q", got)
	}

	s.config = nil
	if got := s.adminToken(); got != "" {
		t.Fatalf("expected empty when config nil, got %q", got)
	}
}
