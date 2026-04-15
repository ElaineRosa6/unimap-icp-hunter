package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleImportURLs_GetMethod_Returns405(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/monitor/import", nil)
	s.handleImportURLs(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleImportURLs_NoFile_Returns400(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/monitor/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	s.handleImportURLs(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleImportURLs_UnsupportedFormat_Returns400(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.pdf")
	part.Write([]byte("some data"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/monitor/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	s.handleImportURLs(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported format, got %d", rec.Code)
	}
}

func TestHandleImportURLs_TXTFile_Success(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "urls.txt")
	part.Write([]byte("http://example.com\nhttps://test.org\n\ninvalid\n"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/monitor/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	s.handleImportURLs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if int(resp["total"].(float64)) < 2 {
		t.Fatalf("expected at least 2 URLs, got %v", resp["total"])
	}
}

func TestHandleImportURLs_CSVFile_Success(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "urls.csv")
	part.Write([]byte("url\nhttp://example.com\nhttps://test.org\n"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/monitor/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	s.handleImportURLs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if int(resp["total"].(float64)) < 2 {
		t.Fatalf("expected at least 2 URLs, got %v", resp["total"])
	}
}

func TestParseCSVFile_SkipHeader(t *testing.T) {
	data := "url\nhttp://example.com\nhttps://test.org\n"
	urls, err := parseCSVFile(strings.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d: %v", len(urls), urls)
	}
}

func TestParseCSVFile_NoHeader(t *testing.T) {
	data := "http://example.com\nhttps://test.org\n"
	urls, err := parseCSVFile(strings.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d: %v", len(urls), urls)
	}
}

func TestParseTXTFile(t *testing.T) {
	data := "http://example.com\n\nhttps://test.org\n\n"
	urls, err := parseTXTFile(strings.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d: %v", len(urls), urls)
	}
}

func TestFilterValidURLs(t *testing.T) {
	urls := []string{
		"http://example.com",
		"https://test.org/path",
		"invalid",
		"",
		"http://example.com", // duplicate
		"  ",
		"http://foo.bar:8080/api",
	}
	valid := filterValidURLs(urls)

	// 有效且去重
	if len(valid) != 4 {
		t.Fatalf("expected 4 valid unique URLs, got %d: %v", len(valid), valid)
	}
}

func TestFilterValidURLs_Empty(t *testing.T) {
	valid := filterValidURLs(nil)
	if valid != nil {
		t.Fatalf("expected nil for empty input")
	}
}

func TestURLPattern_ValidURLs(t *testing.T) {
	testURLs := []string{
		"http://example.com",
		"https://example.com",
		"http://localhost:8080",
		"https://example.com/path/to/resource",
		"example.com",
		"http://192.168.1.1:443",
	}
	for _, u := range testURLs {
		if !reURLPattern.MatchString(u) {
			t.Fatalf("expected %q to match URL pattern", u)
		}
	}
}

func TestURLPattern_InvalidURLs(t *testing.T) {
	testURLs := []string{
		"ftp://example.com",
		"mailto://test@example.com",
		"   ",
		"just spaces    ",
	}
	for _, u := range testURLs {
		if reURLPattern.MatchString(u) {
			t.Fatalf("expected %q NOT to match URL pattern", u)
		}
	}
}

func TestHandleURLReachability_GetMethod_Returns405(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/monitor/reachability", nil)
	s.handleURLReachability(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleURLReachability_EmptyURLs_Returns400(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{"urls": []string{}})
	req := httptest.NewRequest(http.MethodPost, "/api/monitor/reachability", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleURLReachability(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleURLReachability_NoMonitorApp_Returns503(t *testing.T) {
	s := &Server{
		monitorApp: nil,
	}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{"urls": []string{"http://example.com"}})
	req := httptest.NewRequest(http.MethodPost, "/api/monitor/reachability", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleURLReachability(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleURLPortScan_GetMethod_Returns405(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/monitor/portscan", nil)
	s.handleURLPortScan(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleURLPortScan_EmptyURLs_Returns400(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{"urls": []string{}})
	req := httptest.NewRequest(http.MethodPost, "/api/monitor/portscan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleURLPortScan(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleURLPortScan_NoMonitorApp_Returns503(t *testing.T) {
	s := &Server{
		monitorApp: nil,
	}
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]interface{}{"urls": []string{"http://example.com"}})
	req := httptest.NewRequest(http.MethodPost, "/api/monitor/portscan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleURLPortScan(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleMonitorPage(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/monitor", nil)
	s.handleMonitorPage(rec, req)

	// 模板可能不存在，但不应 panic
}
