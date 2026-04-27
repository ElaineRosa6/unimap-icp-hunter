package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/config"
)

// ============================================================
// handleImportCookieJSON tests (supplementary, non-duplicate)
// ============================================================

func TestHandleImportCookieJSON_MissingEngine(t *testing.T) {
	cfg := &config.Config{}
	s := &Server{config: cfg}
	body := strings.NewReader("engine=&cookie_json=[]")
	req := httptest.NewRequest(http.MethodPost, "/api/cookies", body)
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://localhost:8448")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.handleImportCookieJSON(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid_request") {
		t.Fatalf("expected 'invalid_request' in body, got %q", w.Body.String())
	}
}

func TestHandleImportCookieJSON_InvalidJSON(t *testing.T) {
	cfg := &config.Config{}
	s := &Server{config: cfg}
	body := strings.NewReader("engine=fofa&cookie_json=not-json")
	req := httptest.NewRequest(http.MethodPost, "/api/cookies", body)
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://localhost:8448")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.handleImportCookieJSON(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid_cookie_json") {
		t.Fatalf("expected 'invalid_cookie_json' in body, got %q", w.Body.String())
	}
}

func TestHandleImportCookieJSON_EmptyCookieSet(t *testing.T) {
	cfg := &config.Config{}
	s := &Server{config: cfg}
	body := strings.NewReader("engine=fofa&cookie_json=[]")
	req := httptest.NewRequest(http.MethodPost, "/api/cookies", body)
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://localhost:8448")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.handleImportCookieJSON(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "empty_cookie_set") {
		t.Fatalf("expected 'empty_cookie_set' in body, got %q", w.Body.String())
	}
}

func TestHandleImportCookieJSON_UnsupportedEngine(t *testing.T) {
	cfg := &config.Config{}
	s := &Server{config: cfg}
	body := strings.NewReader(`engine=unknown&cookie_json=[{"name":"test","value":"val","domain":".example.com"}]`)
	req := httptest.NewRequest(http.MethodPost, "/api/cookies", body)
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://localhost:8448")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.handleImportCookieJSON(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "unsupported_engine") {
		t.Fatalf("expected 'unsupported_engine' in body, got %q", w.Body.String())
	}
}

// ============================================================
// applyCookiesFromRequest tests
// ============================================================

func TestApplyCookiesFromRequest_NilConfig(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// Should not panic with nil config
	s.applyCookiesFromRequest(req)
}

// ============================================================
// parseEnginesParam supplementary tests
// ============================================================

func TestParseEnginesParam_WhitespaceTrimmed(t *testing.T) {
	u := "/?engines=" + url.QueryEscape(" fofa , hunter ")
	req := httptest.NewRequest(http.MethodGet, u, nil)
	got := parseEnginesParam(req)
	for _, e := range got {
		if e != strings.TrimSpace(e) {
			t.Errorf("engine %q should be trimmed", e)
		}
	}
}

func TestParseEnginesParam_EmptyEntriesRemoved(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?engines=fofa,,hunter,", nil)
	got := parseEnginesParam(req)
	for _, e := range got {
		if e == "" {
			t.Fatal("expected empty entries to be removed")
		}
	}
}

// ============================================================
// validateQueryInput supplementary tests
// ============================================================

func TestValidateQueryInput_Empty(t *testing.T) {
	err := validateQueryInput("")
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestValidateQueryInput_Valid(t *testing.T) {
	err := validateQueryInput("port:80")
	if err != nil {
		t.Fatalf("expected no error for valid query, got %v", err)
	}
}

// ============================================================
// cookiesToHeader tests
// ============================================================

func TestCookiesToHeader_Empty(t *testing.T) {
	got := cookiesToHeader(nil)
	if got != "" {
		t.Fatalf("expected empty string for nil cookies, got %q", got)
	}
}

// ============================================================
// convertConfigCookies tests
// ============================================================

func TestConvertConfigCookies_Nil(t *testing.T) {
	got := convertConfigCookies(nil)
	if len(got) != 0 {
		t.Fatalf("expected empty slice for nil, got %d cookies", len(got))
	}
}

func TestConvertConfigCookies_Empty(t *testing.T) {
	got := convertConfigCookies([]config.Cookie{})
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %d cookies", len(got))
	}
}

func TestConvertConfigCookies_Single(t *testing.T) {
	input := []config.Cookie{
		{Name: "session", Value: "abc123", Domain: ".example.com", Path: "/", Secure: true},
	}
	got := convertConfigCookies(input)
	if len(got) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(got))
	}
	if got[0].Name != "session" || got[0].Value != "abc123" {
		t.Fatalf("unexpected cookie: %+v", got[0])
	}
}

// ============================================================
// handleVerifyCookies supplementary tests
// ============================================================

func TestHandleVerifyCookies_WrongMethod(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/cookies/verify", nil)
	w := httptest.NewRecorder()
	s.handleVerifyCookies(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleVerifyCookies_InvalidQuery(t *testing.T) {
	cfg := &config.Config{}
	s := &Server{config: cfg}
	body := strings.NewReader("query=")
	req := httptest.NewRequest(http.MethodPost, "/api/cookies/verify", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.handleVerifyCookies(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid_query") {
		t.Fatalf("expected 'invalid_query' in body, got %q", w.Body.String())
	}
}

// ============================================================
// handleCookieLoginStatus tests
// ============================================================

func TestHandleCookieLoginStatus_Success(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/cookies/login-status", nil)
	w := httptest.NewRecorder()
	s.handleCookieLoginStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"success":true`) {
		t.Fatalf("expected 'success':true in body, got %q", w.Body.String())
	}
}

func TestHandleCookieLoginStatus_WrongMethod(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/cookies/login-status", nil)
	w := httptest.NewRecorder()
	s.handleCookieLoginStatus(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// ============================================================
// hasCookies tests
// ============================================================

func TestHasCookies_Nil(t *testing.T) {
	if hasCookies(nil) {
		t.Fatal("expected false for nil cookies")
	}
}

func TestHasCookies_Empty(t *testing.T) {
	if hasCookies([]config.Cookie{}) {
		t.Fatal("expected false for empty cookies")
	}
}

func TestHasCookies_WithName(t *testing.T) {
	cookies := []config.Cookie{{Name: "session", Value: "abc"}}
	if !hasCookies(cookies) {
		t.Fatal("expected true when cookie name is present")
	}
}

func TestHasCookies_AllEmptyNames(t *testing.T) {
	cookies := []config.Cookie{{Name: "", Value: "abc"}}
	if hasCookies(cookies) {
		t.Fatal("expected false when all names are empty")
	}
}

// ============================================================
// verifyEngineSession unsupported engine test
// ============================================================

func TestVerifyEngineSession_UnknownEngine(t *testing.T) {
	cfg := &config.Config{}
	s := &Server{config: cfg}
	ok, _, hint, err := s.verifyEngineSession(nil, "cdp", "unknown_engine", "test")
	if ok {
		t.Fatal("expected false for unknown engine")
	}
	// CDP mode returns specific hints for missing cookies vs unsupported engine
	if err == nil {
		t.Fatal("expected error for unknown engine")
	}
	_ = hint // hint varies by implementation
}
