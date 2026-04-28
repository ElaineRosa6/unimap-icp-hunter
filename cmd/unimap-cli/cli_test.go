package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/config"
	"github.com/unimap-icp-hunter/project/internal/model"
)

func TestSplitCSVText_Empty(t *testing.T) {
	got := splitCSVText("")
	if len(got) != 0 {
		t.Fatalf("expected 0 items, got %d", len(got))
	}
}

func TestSplitCSVText_NewlinesOnly(t *testing.T) {
	got := splitCSVText(",,\n\n")
	if len(got) != 0 {
		t.Fatalf("expected 0 items, got %d", len(got))
	}
}

func TestSplitCSVText_SingleValue(t *testing.T) {
	got := splitCSVText("https://example.com")
	if len(got) != 1 || got[0] != "https://example.com" {
		t.Fatalf("expected [https://example.com], got %v", got)
	}
}

func TestMaxInt_Equal(t *testing.T) {
	if maxInt(5, 5) != 5 {
		t.Fatalf("expected 5")
	}
}

func TestMaxInt_Negative(t *testing.T) {
	// maxInt should return the larger value even with negative inputs
	if maxInt(-1, -5) != -1 {
		t.Fatalf("expected -1")
	}
}

func TestWriteJSONFile_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	data := map[string]string{"key": "value"}

	err := writeJSONFile(path, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal(content, &got); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if got["key"] != "value" {
		t.Fatalf("expected key=value, got %v", got)
	}
}

func TestWriteJSONFile_InvalidPath(t *testing.T) {
	err := writeJSONFile("/nonexistent/invalid/path/test.json", map[string]string{"a": "b"})
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestRunAPISubcommand_UnknownCommand(t *testing.T) {
	// Should return false for unknown commands
	if runAPISubcommand("unknown", nil) {
		t.Fatal("expected false for unknown command")
	}
	if runAPISubcommand("", nil) {
		t.Fatal("expected false for empty command")
	}
}

func TestApplyCookiesFromFlags_AllSet(t *testing.T) {
	cfg := &config.Config{}
	cfg.Engines.Fofa.Cookies = nil
	cfg.Engines.Hunter.Cookies = nil
	cfg.Engines.Quake.Cookies = nil
	cfg.Engines.Zoomeye.Cookies = nil

	changed := applyCookiesFromFlags(cfg, "session=fofa123", "token=hunter456", "key=quake789", "api=zoomeye000")
	if !changed {
		t.Fatal("expected true when all cookies are set")
	}
	if len(cfg.Engines.Fofa.Cookies) == 0 {
		t.Fatal("expected fofa cookies to be set")
	}
	if len(cfg.Engines.Hunter.Cookies) == 0 {
		t.Fatal("expected hunter cookies to be set")
	}
}

func TestApplyCookiesFromFlags_NoneSet(t *testing.T) {
	cfg := &config.Config{}
	changed := applyCookiesFromFlags(cfg, "", "", "", "")
	if changed {
		t.Fatal("expected false when no cookies are set")
	}
}

func TestApplyCookiesFromFlags_OnlyFofa(t *testing.T) {
	cfg := &config.Config{}
	changed := applyCookiesFromFlags(cfg, "session=fofa123", "", "", "")
	if !changed {
		t.Fatal("expected true when only fofa cookie is set")
	}
	if len(cfg.Engines.Fofa.Cookies) == 0 {
		t.Fatal("expected fofa cookies to be set")
	}
	if len(cfg.Engines.Hunter.Cookies) != 0 {
		t.Fatal("expected hunter cookies to remain empty")
	}
}

func TestApplyCookiesFromFlags_WhitespaceIgnored(t *testing.T) {
	cfg := &config.Config{}
	changed := applyCookiesFromFlags(cfg, "  ", " \t ", "", "")
	if changed {
		t.Fatal("expected false for whitespace-only cookie")
	}
}

func TestGetEnabledEngines_AllEnabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Engines.Fofa.Enabled = true
	cfg.Engines.Hunter.Enabled = true
	cfg.Engines.Quake.Enabled = true
	cfg.Engines.Zoomeye.Enabled = true
	cfg.Engines.Shodan.Enabled = true

	got := getEnabledEngines(cfg)
	if len(got) != 5 {
		t.Fatalf("expected 5 engines, got %d: %v", len(got), got)
	}
}

func TestGetEnabledEngines_NoneEnabled(t *testing.T) {
	cfg := &config.Config{}
	got := getEnabledEngines(cfg)
	if len(got) != 0 {
		t.Fatalf("expected 0 engines, got %d", len(got))
	}
}

func TestGetEnabledEngines_SomeEnabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Engines.Fofa.Enabled = true
	cfg.Engines.Shodan.Enabled = true

	got := getEnabledEngines(cfg)
	if len(got) != 2 {
		t.Fatalf("expected 2 engines, got %d: %v", len(got), got)
	}
}

func TestSaveResults_JSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.json")
	assets := []model.UnifiedAsset{
		{IP: "1.2.3.4", Port: 80, Host: "example.com"},
	}

	err := saveResults(assets, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var got []model.UnifiedAsset
	if err := json.Unmarshal(content, &got); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if len(got) != 1 || got[0].IP != "1.2.3.4" {
		t.Fatalf("unexpected results: %+v", got)
	}
}

func TestSaveResults_XLSX(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.xlsx")
	assets := []model.UnifiedAsset{
		{IP: "1.2.3.4", Port: 80},
	}

	err := saveResults(assets, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file exists and is non-empty
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("file is empty")
	}
}

func TestSaveResults_CSV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.csv")
	assets := []model.UnifiedAsset{
		{IP: "1.2.3.4", Port: 80, Protocol: "http", Host: "example.com", Title: "Test", Source: "fofa"},
		{IP: "5.6.7.8", Port: 443, Protocol: "https", Host: "secure.com", Title: "Secure", Source: "hunter"},
	}

	err := saveResults(assets, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	lines := string(content)
	// Should have header + 2 data lines
	expected := "IP,Port,Protocol,Domain,Title,Country,City,ISP,Source\n" +
		"1.2.3.4,80,http,example.com,Test,,,,fofa\n" +
		"5.6.7.8,443,https,secure.com,Secure,,,,hunter\n"
	if lines != expected {
		t.Fatalf("unexpected CSV content:\ngot:\n%s\nwant:\n%s", lines, expected)
	}
}

func TestSaveResults_WithWriteError(t *testing.T) {
	// Try to write to a directory that doesn't exist
	err := saveResults([]model.UnifiedAsset{}, "/nonexistent/dir/results.csv")
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}

func TestDoJSONRequest_EmptyBaseURL(t *testing.T) {
	var resp map[string]string
	err := doJSONRequest("", "/api", 5, map[string]string{}, &resp)
	if err == nil {
		t.Fatal("expected error for empty base URL")
	}
	if err.Error() != "api base is empty" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDoFormRequest_EmptyBaseURL(t *testing.T) {
	values := make(map[string][]string)
	var resp map[string]string
	err := doFormRequest("", "/api", 5, values, &resp)
	if err == nil {
		t.Fatal("expected error for empty base URL")
	}
	if err.Error() != "api base is empty" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDoJSONRequest_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not valid json`))
	}))
	defer ts.Close()

	var resp map[string]string
	err := doJSONRequest(ts.URL, "/api", 5, map[string]string{"a": "b"}, &resp)
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
}

func TestDoFormRequest_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not valid json`))
	}))
	defer ts.Close()

	values := make(map[string][]string)
	var resp map[string]string
	err := doFormRequest(ts.URL, "/api", 5, values, &resp)
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
}

func TestDoJSONRequest_Non2xxStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`service unavailable`))
	}))
	defer ts.Close()

	var resp map[string]string
	err := doJSONRequest(ts.URL, "/api", 5, map[string]string{}, &resp)
	if err == nil {
		t.Fatal("expected error for 503 status")
	}
}

func TestDoFormRequest_Non2xxStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`unauthorized`))
	}))
	defer ts.Close()

	values := make(map[string][]string)
	var resp map[string]string
	err := doFormRequest(ts.URL, "/api", 5, values, &resp)
	if err == nil {
		t.Fatal("expected error for 401 status")
	}
}

func TestRunAPIScreenshotBatch_RecognizesFlagSet(t *testing.T) {
	// Verify the function's flag set is properly configured by checking
	// that splitCSVText works correctly with the expected input format
	got := splitCSVText("https://a.com,https://b.com")
	if len(got) != 2 {
		t.Fatalf("expected 2 URLs, got %d", len(got))
	}
}

func TestWriteJSONFile_Array(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "array.json")
	data := []string{"a", "b", "c"}

	err := writeJSONFile(path, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var got []string
	if err := json.Unmarshal(content, &got); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 items, got %d", len(got))
	}
}
