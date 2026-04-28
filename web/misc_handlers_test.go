package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCSVFile_Valid(t *testing.T) {
	csvData := "url\nhttps://example.com\nhttps://test.com\n"
	urls, err := parseCSVFile(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d: %v", len(urls), urls)
	}
}

func TestParseCSVFile_Empty(t *testing.T) {
	urls, err := parseCSVFile(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 0 {
		t.Fatalf("expected 0 URLs, got %d", len(urls))
	}
}

func TestParseCSVFile_WithChineseHeader(t *testing.T) {
	csvData := "网址\nhttps://example.com\n"
	urls, err := parseCSVFile(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL, got %d: %v", len(urls), urls)
	}
}

func TestParseCSVFile_WhitespaceEntries(t *testing.T) {
	csvData := "url\n  https://example.com  \n\nhttps://test.com\n"
	urls, err := parseCSVFile(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d: %v", len(urls), urls)
	}
}

func TestParseExcelFile_InvalidData(t *testing.T) {
	// Passing plain text instead of Excel should fail
	_, err := parseExcelFile(strings.NewReader("not an excel file"))
	if err == nil {
		t.Fatal("expected error for invalid Excel data")
	}
}

func TestFilterValidURLs_Dedup(t *testing.T) {
	input := []string{"https://a.com", "https://b.com", "https://a.com", ""}
	got := filterValidURLs(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 URLs (dedup), got %d: %v", len(got), got)
	}
}

func TestFilterValidURLs_TrimSpace(t *testing.T) {
	input := []string{"  https://a.com  ", "https://b.com"}
	got := filterValidURLs(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 URLs, got %d: %v", len(got), got)
	}
}

func TestFilterValidURLs_AllEmpty(t *testing.T) {
	input := []string{"", "  ", ""}
	got := filterValidURLs(input)
	if len(got) != 0 {
		t.Fatalf("expected 0 URLs, got %d", len(got))
	}
}

func TestHandleMetrics_GetOnly(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/metrics", nil)
	s.handleMetrics(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST, got %d", rec.Code)
	}
}

func TestIsWebRoot_Valid(t *testing.T) {
	// Create temp dir with templates and static subdirs
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "templates"), 0755)
	os.Mkdir(filepath.Join(dir, "static"), 0755)

	if !isWebRoot(dir) {
		t.Fatal("expected true for valid web root")
	}
}

func TestIsWebRoot_MissingTemplates(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "static"), 0755)

	if isWebRoot(dir) {
		t.Fatal("expected false for missing templates dir")
	}
}

func TestIsWebRoot_MissingStatic(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "templates"), 0755)

	if isWebRoot(dir) {
		t.Fatal("expected false for missing static dir")
	}
}

func TestIsWebRoot_NonExistentDir(t *testing.T) {
	if isWebRoot("/nonexistent/path/that/does/not/exist") {
		t.Fatal("expected false for non-existent dir")
	}
}

func TestStatusRecorder_WriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, statusCode: http.StatusOK}
	sr.WriteHeader(http.StatusNotFound)

	if sr.statusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", sr.statusCode)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected underlying recorder to have 404, got %d", rec.Code)
	}
}

func TestCORSMiddleware_ExposedHeaders(t *testing.T) {
	mw := corsMiddleware([]string{"*"}, nil, nil, []string{"X-Custom"}, false, 0)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Expose-Headers") != "X-Custom" {
		t.Fatalf("expected expose header, got %q", rec.Header().Get("Access-Control-Expose-Headers"))
	}
}

func TestDecodeJSONBody_TooLarge(t *testing.T) {
	// Verify that a very large body results in an error (may be 400 or 413
	// depending on how the decoder encounters the limit)
	largeData := bytes.NewReader(bytes.Repeat([]byte("a"), 20*1024*1024))
	req := httptest.NewRequest(http.MethodPost, "/api", largeData)
	rec := httptest.NewRecorder()
	var dst map[string]string

	if decodeJSONBody(rec, req, &dst) {
		t.Fatal("expected false for too-large body")
	}
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 400 or 413, got %d", rec.Code)
	}
}

func TestRequireTrustedRequest_Forbidden(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api", nil)
	req.Header.Set("Origin", "https://evil.com")
	req.Header.Set("Referer", "https://evil.com/page")
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	if requireTrustedRequest(rec, req, []string{"https://good.com"}) {
		t.Fatal("expected false for disallowed origin")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
