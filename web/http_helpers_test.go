package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON_EncodesPayload(t *testing.T) {
	rec := httptest.NewRecorder()
	payload := map[string]string{"key": "value"}
	writeJSON(rec, http.StatusOK, payload)

	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", rec.Header().Get("Content-Type"))
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var got map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if got["key"] != "value" {
		t.Fatalf("expected key=value, got %v", got)
	}
}

func TestWriteAPIError_ReturnsErrorEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	writeAPIError(rec, http.StatusBadRequest, "bad_input", "invalid input", nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var got map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if got["success"] != false {
		t.Fatalf("expected success=false, got %v", got["success"])
	}
	errMap := got["error"].(map[string]interface{})
	if errMap["code"] != "bad_input" {
		t.Fatalf("expected code=bad_input, got %v", errMap["code"])
	}
	if errMap["message"] != "invalid input" {
		t.Fatalf("expected message=invalid input, got %v", errMap["message"])
	}
}

func TestRequireMethod_AllowsMatching(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api", nil)
	rec := httptest.NewRecorder()
	if !requireMethod(rec, req, http.MethodPost) {
		t.Fatal("expected true for matching method")
	}
}

func TestRequireMethod_RejectsNonMatching(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	rec := httptest.NewRecorder()
	if requireMethod(rec, req, http.MethodPost) {
		t.Fatal("expected false for non-matching method")
	}
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestDecodeJSONBody_MissingBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api", nil)
	rec := httptest.NewRecorder()
	var dst map[string]string
	if decodeJSONBody(rec, req, &dst) {
		t.Fatal("expected false for empty body")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDecodeJSONBody_ValidJSON(t *testing.T) {
	body := bytes.NewReader([]byte(`{"name":"test"}`))
	req := httptest.NewRequest(http.MethodPost, "/api", body)
	rec := httptest.NewRecorder()
	var dst map[string]string
	if !decodeJSONBody(rec, req, &dst) {
		t.Fatalf("expected true, got response %d: %s", rec.Code, rec.Body.String())
	}
	if dst["name"] != "test" {
		t.Fatalf("expected name=test, got %q", dst["name"])
	}
}

func TestDecodeJSONBody_InvalidJSON(t *testing.T) {
	body := bytes.NewReader([]byte(`{not json}`))
	req := httptest.NewRequest(http.MethodPost, "/api", body)
	rec := httptest.NewRecorder()
	var dst map[string]string
	if decodeJSONBody(rec, req, &dst) {
		t.Fatal("expected false for invalid JSON")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDecodeJSONBody_ExtraData(t *testing.T) {
	body := bytes.NewReader([]byte(`{"a":1}{"b":2}`))
	req := httptest.NewRequest(http.MethodPost, "/api", body)
	rec := httptest.NewRecorder()
	var dst map[string]int
	if decodeJSONBody(rec, req, &dst) {
		t.Fatal("expected false for multiple JSON objects")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestIsSameHostURL_Matches(t *testing.T) {
	if !isSameHostURL("http://example.com/path", "example.com") {
		t.Fatal("expected true for same host")
	}
}

func TestIsSameHostURL_Different(t *testing.T) {
	if isSameHostURL("http://other.com/path", "example.com") {
		t.Fatal("expected false for different host")
	}
}

func TestIsSameHostURL_Empty(t *testing.T) {
	if isSameHostURL("", "example.com") {
		t.Fatal("expected false for empty URL")
	}
}

func TestIsSameHostURL_InvalidURL(t *testing.T) {
	if isSameHostURL("://invalid", "example.com") {
		t.Fatal("expected false for invalid URL")
	}
}

func TestNormalizeOrigin_TrailingSlash(t *testing.T) {
	got := normalizeOrigin("https://example.com/")
	want := "https://example.com"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestOriginAllowedByList_StarWildcard(t *testing.T) {
	if !originAllowedByList("https://any.com", []string{"*"}) {
		t.Fatal("expected true for star wildcard")
	}
}

func TestOriginAllowedByList_ExactMatch(t *testing.T) {
	if !originAllowedByList("https://example.com", []string{"https://example.com"}) {
		t.Fatal("expected true for exact match")
	}
}

func TestOriginAllowedByList_NotFound(t *testing.T) {
	if originAllowedByList("https://evil.com", []string{"https://example.com"}) {
		t.Fatal("expected false for non-matching origin")
	}
}

func TestOriginAllowedByList_EmptyOrigin(t *testing.T) {
	if originAllowedByList("", []string{"https://example.com"}) {
		t.Fatal("expected false for empty origin")
	}
}

func TestIsOriginAllowed_FromList(t *testing.T) {
	if !isOriginAllowed("https://allowed.com", "other.com", []string{"https://allowed.com"}) {
		t.Fatal("expected true for allowed origin")
	}
}

func TestIsTrustedRequest_AllowedOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Host = "example.com"
	if !isTrustedRequest(req, nil) {
		t.Fatal("expected true for same-host origin")
	}
}

func TestIsTrustedRequest_AllowedReferer(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Referer", "https://example.com/page")
	req.Host = "example.com"
	if !isTrustedRequest(req, nil) {
		t.Fatal("expected true for same-host referer")
	}
}

func TestRequestSizeLimitMiddleware_SkipsGet(t *testing.T) {
	mw := requestSizeLimitMiddleware(100)
	handlerCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.ContentLength = 200
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Fatal("expected handler to be called for GET")
	}
}

func TestRequestSizeLimitMiddleware_SkipsWebSocket(t *testing.T) {
	mw := requestSizeLimitMiddleware(100)
	handlerCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	}))

	body := bytes.NewReader([]byte("large-payload"))
	req := httptest.NewRequest(http.MethodPost, "/api", body)
	req.ContentLength = 200
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Fatal("expected handler to be called for WebSocket")
	}
}

func TestRequestSizeLimitMiddleware_DefaultLimit(t *testing.T) {
	mw := requestSizeLimitMiddleware(0)
	handlerCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	}))

	body := bytes.NewReader([]byte("small"))
	req := httptest.NewRequest(http.MethodPost, "/api", body)
	req.ContentLength = 5
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Fatal("expected handler to be called with default limit")
	}
}

func TestCORSMiddleware_SetsHeaders(t *testing.T) {
	mw := corsMiddleware([]string{"https://example.com"}, nil, nil, nil, true, 3600)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Fatalf("expected ACAO header, got %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("expected credentials header")
	}
	if rec.Header().Get("Vary") != "Origin" {
		t.Fatal("expected Vary header")
	}
}

func TestCORSMiddleware_Preflight(t *testing.T) {
	mw := corsMiddleware([]string{"https://example.com"}, nil, nil, nil, false, 3600)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for preflight")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatal("expected Allow-Methods header")
	}
	if rec.Header().Get("Access-Control-Max-Age") != "3600" {
		t.Fatalf("expected Max-Age=3600, got %q", rec.Header().Get("Access-Control-Max-Age"))
	}
}

func TestCORSMiddleware_PreflightForbidden(t *testing.T) {
	mw := corsMiddleware([]string{"https://example.com"}, nil, nil, nil, false, 0)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestCORSMiddleware_NonOriginRequest(t *testing.T) {
	mw := corsMiddleware([]string{"https://example.com"}, nil, nil, nil, false, 0)
	handlerCalled := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Fatal("expected handler to be called for non-origin request")
	}
}

func TestExtractBearerToken_Valid(t *testing.T) {
	got := extractBearerToken("Bearer abc123")
	if got != "abc123" {
		t.Fatalf("expected abc123, got %q", got)
	}
}

func TestExtractBearerToken_Empty(t *testing.T) {
	got := extractBearerToken("")
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestExtractBearerToken_NoBearer(t *testing.T) {
	got := extractBearerToken("Token abc123")
	if got != "" {
		t.Fatalf("expected empty for non-Bearer prefix, got %q", got)
	}
}

func TestExtractBearerToken_Whitespace(t *testing.T) {
	got := extractBearerToken("  Bearer  token  ")
	if got != "token" {
		t.Fatalf("expected 'token', got %q", got)
	}
}
