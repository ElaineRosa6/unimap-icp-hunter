package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ===== normalizePathToken =====

func TestNormalizePathToken(t *testing.T) {
	svc := NewScreenshotAppService("./screenshots")
	tests := []struct {
		input string
		want  string
	}{
		{"valid_name", "valid_name"},
		{"batch123", "batch123"},
		{"", ""},
		{".", ""},
		{"..", ""},
		{"../etc", ""},
		{"foo/bar", ""},
		{"foo\\bar", ""},
		{"  spaced  ", "spaced"},
	}
	for _, tt := range tests {
		got := svc.normalizePathToken(tt.input)
		if got != tt.want {
			t.Errorf("normalizePathToken(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ===== buildSearchEngineResultURL =====

func TestBuildSearchEngineResultURL(t *testing.T) {
	tests := []struct {
		engine     string
		query      string
		wantEngine string
	}{
		{"fofa", "test", "fofa.info"},
		{"hunter", "test", "hunter.qianxin.com"},
		{"quake", "test", "quake.360.net"},
		{"zoomeye", "test", "zoomeye.hk"},
		{"unknown", "test", ""},
		{"", "test", ""},
	}
	for _, tt := range tests {
		got := buildSearchEngineResultURL(tt.engine, tt.query)
		if tt.wantEngine != "" && !strings.Contains(got, tt.wantEngine) {
			t.Errorf("buildSearchEngineURL(%q, %q) = %q, should contain %q", tt.engine, tt.query, got, tt.wantEngine)
		}
		if tt.wantEngine == "" && got != "" {
			t.Errorf("buildSearchEngineURL(%q, %q) = %q, want empty", tt.engine, tt.query, got)
		}
	}
}

// ===== buildTargetCaptureURL =====

func TestBuildTargetCaptureURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		ip       string
		port     string
		protocol string
		want     string
		wantErr  bool
	}{
		{"url provided", "http://example.com", "", "", "", "http://example.com", false},
		{"ip only", "", "1.2.3.4", "", "", "http://1.2.3.4", false},
		{"ip + port 443", "", "1.2.3.4", "443", "", "https://1.2.3.4", false},
		{"ip + port 80", "", "1.2.3.4", "80", "", "http://1.2.3.4", false},
		{"ip + port 8080", "", "1.2.3.4", "8080", "", "http://1.2.3.4:8080", false},
		{"ip + protocol https", "", "1.2.3.4", "", "https", "https://1.2.3.4", false},
		{"ip + protocol + port", "", "1.2.3.4", "9443", "https", "https://1.2.3.4:9443", false},
		{"empty url and ip", "", "", "", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildTargetCaptureURL(tt.url, tt.ip, tt.port, tt.protocol)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got = %q, want %q", got, tt.want)
			}
		})
	}
}

// ===== normalizeBridgeTargetURL =====

func TestNormalizeBridgeTargetURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"  ", ""},
		{"example.com", "http://example.com"},
		{"http://example.com", "http://example.com"},
		{"https://example.com/path", "https://example.com/path"},
		{"  example.com  ", "http://example.com"},
	}
	for _, tt := range tests {
		got := normalizeBridgeTargetURL(tt.input)
		if got != tt.want {
			t.Errorf("normalizeBridgeTargetURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ===== CaptureSearchEngineResult validation =====

func TestCaptureSearchEngineResult_MissingParams(t *testing.T) {
	svc := NewScreenshotAppService("./screenshots")

	_, _, _, _, err := svc.CaptureSearchEngineResult(context.Background(), nil, "", "test", "q1")
	if err == nil {
		t.Fatal("expected error for empty engine")
	}

	_, _, _, _, err = svc.CaptureSearchEngineResult(context.Background(), nil, "fofa", "", "q1")
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestCaptureSearchEngineResult_NoProvider(t *testing.T) {
	svc := NewScreenshotAppService("./screenshots")
	_, _, _, _, err := svc.CaptureSearchEngineResult(context.Background(), nil, "fofa", "test", "q1")
	if err == nil {
		t.Fatal("expected error when no provider and no manager")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("error should mention 'not initialized': %v", err)
	}
}

func TestCaptureSearchEngineResult_QueryIDGenerated(t *testing.T) {
	svc := NewScreenshotAppServiceWithProvider("./screenshots", &mockScreenshotProvider{})
	_, _, _, queryID, err := svc.CaptureSearchEngineResult(context.Background(), nil, "fofa", "test", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if queryID == "" {
		t.Error("expected auto-generated queryID")
	}
}

func TestCaptureSearchEngineResult_WithProxy(t *testing.T) {
	svc := NewScreenshotAppServiceWithProvider("./screenshots", &mockScreenshotProvider{})
	_, _, _, _, err := svc.CaptureSearchEngineResultWithProxy(context.Background(), nil, "fofa", "test", "q1", "http://proxy:8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ===== CaptureTargetWebsite validation =====

func TestCaptureTargetWebsite_MissingParams(t *testing.T) {
	svc := NewScreenshotAppService("./screenshots")

	_, _, _, _, _, _, err := svc.CaptureTargetWebsite(context.Background(), nil, "", "", "", "", "")
	if err == nil {
		t.Fatal("expected error for empty url and ip")
	}
}

func TestCaptureTargetWebsite_NoProvider(t *testing.T) {
	svc := NewScreenshotAppService("./screenshots")
	_, _, _, _, _, _, err := svc.CaptureTargetWebsite(context.Background(), nil, "http://example.com", "", "", "", "")
	if err == nil {
		t.Fatal("expected error when no provider and no manager")
	}
}

func TestCaptureTargetWebsite_QueryIDGenerated(t *testing.T) {
	svc := NewScreenshotAppServiceWithProvider("./screenshots", &mockScreenshotProvider{})
	_, _, _, _, _, queryID, err := svc.CaptureTargetWebsite(context.Background(), nil, "http://example.com", "", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if queryID == "" {
		t.Error("expected auto-generated queryID")
	}
}

// ===== CaptureBatchURLs validation =====

func TestCaptureBatchURLs_EmptyList(t *testing.T) {
	svc := NewScreenshotAppService("./screenshots")
	_, err := svc.CaptureBatchURLs(context.Background(), nil, BatchURLsRequest{})
	if err == nil {
		t.Fatal("expected error for empty URLs")
	}
}

func TestCaptureBatchURLs_TooMany(t *testing.T) {
	svc := NewScreenshotAppService("./screenshots")
	urls := make([]string, 101)
	for i := range urls {
		urls[i] = "http://example.com"
	}
	_, err := svc.CaptureBatchURLs(context.Background(), nil, BatchURLsRequest{URLs: urls})
	if err == nil {
		t.Fatal("expected error for too many URLs")
	}
}

func TestCaptureBatchURLs_DefaultsApplied(t *testing.T) {
	svc := NewScreenshotAppServiceWithProvider("./screenshots", &mockScreenshotProvider{})
	req := BatchURLsRequest{URLs: []string{"http://example.com"}}
	resp, err := svc.CaptureBatchURLs(context.Background(), nil, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.BatchID == "" {
		t.Error("expected auto-generated batch ID")
	}
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Total)
	}
}

func TestCaptureBatchURLs_DefaultConcurrency(t *testing.T) {
	svc := NewScreenshotAppServiceWithProvider("./screenshots", &mockScreenshotProvider{})
	req := BatchURLsRequest{URLs: []string{"http://example.com"}, Concurrency: -1}
	resp, err := svc.CaptureBatchURLs(context.Background(), nil, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Total)
	}
}

// ===== ListBatches / ListBatchFiles / DeleteBatch / DeleteFile =====

func TestListBatches_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	svc := NewScreenshotAppService(dir)
	batches, err := svc.ListBatches()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(batches) != 0 {
		t.Errorf("expected 0 batches, got %d", len(batches))
	}
}

func TestListBatches_NonExistentDir(t *testing.T) {
	svc := NewScreenshotAppService("/nonexistent/path/that/does/not/exist")
	batches, err := svc.ListBatches()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(batches) != 0 {
		t.Errorf("expected 0 batches for non-existent dir, got %d", len(batches))
	}
}

func TestListBatches_WithDirs(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "batch1"), 0755)
	os.MkdirAll(filepath.Join(dir, "batch2"), 0755)
	os.WriteFile(filepath.Join(dir, "batch1", "screenshot.png"), []byte("data"), 0644)
	os.WriteFile(filepath.Join(dir, "batch2", "screenshot.png"), []byte("data"), 0644)
	os.WriteFile(filepath.Join(dir, "batch2", "screenshot2.png"), []byte("data"), 0644)

	svc := NewScreenshotAppService(dir)
	batches, err := svc.ListBatches()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(batches) != 2 {
		t.Errorf("expected 2 batches, got %d", len(batches))
	}
}

func TestListBatchFiles_Valid(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "mybatch"), 0755)
	os.WriteFile(filepath.Join(dir, "mybatch", "shot.png"), []byte("data"), 0644)

	svc := NewScreenshotAppService(dir)
	files, err := svc.ListBatchFiles("mybatch", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}
	if files[0].Name != "shot.png" {
		t.Errorf("file name = %q, want %q", files[0].Name, "shot.png")
	}
}

func TestListBatchFiles_InvalidName(t *testing.T) {
	dir := t.TempDir()
	svc := NewScreenshotAppService(dir)

	_, err := svc.ListBatchFiles("", nil)
	if err == nil {
		t.Fatal("expected error for empty batch name")
	}

	_, err = svc.ListBatchFiles("..", nil)
	if err == nil {
		t.Fatal("expected error for traversal batch name")
	}

	_, err = svc.ListBatchFiles("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for non-existent batch")
	}
}

func TestListBatchFiles_WithPreviewBuilder(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "batch"), 0755)
	os.WriteFile(filepath.Join(dir, "batch", "shot.png"), []byte("data"), 0644)

	svc := NewScreenshotAppService(dir)
	files, err := svc.ListBatchFiles("batch", func(p string) string {
		return "https://preview/" + filepath.Base(p)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if files[0].PreviewURL == "" {
		t.Error("expected preview URL")
	}
	if !strings.Contains(files[0].PreviewURL, "shot.png") {
		t.Errorf("preview URL should contain file name: %s", files[0].PreviewURL)
	}
}

func TestDeleteBatch_Valid(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "todelete"), 0755)
	os.WriteFile(filepath.Join(dir, "todelete", "shot.png"), []byte("data"), 0644)

	svc := NewScreenshotAppService(dir)
	err := svc.DeleteBatch("todelete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "todelete")); !os.IsNotExist(err) {
		t.Error("batch directory should be deleted")
	}
}

func TestDeleteBatch_Invalid(t *testing.T) {
	dir := t.TempDir()
	svc := NewScreenshotAppService(dir)

	err := svc.DeleteBatch("")
	if err == nil {
		t.Fatal("expected error for empty batch name")
	}

	err = svc.DeleteBatch("../etc")
	if err == nil {
		t.Fatal("expected error for traversal batch name")
	}

	err = svc.DeleteBatch("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent batch")
	}
}

func TestDeleteFile_Valid(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "batch"), 0755)
	os.WriteFile(filepath.Join(dir, "batch", "shot.png"), []byte("data"), 0644)

	svc := NewScreenshotAppService(dir)
	err := svc.DeleteFile("batch", "shot.png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "batch", "shot.png")); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

func TestDeleteFile_Invalid(t *testing.T) {
	dir := t.TempDir()
	svc := NewScreenshotAppService(dir)

	err := svc.DeleteFile("", "file.png")
	if err == nil {
		t.Fatal("expected error for empty batch")
	}

	err = svc.DeleteFile("batch", "")
	if err == nil {
		t.Fatal("expected error for empty filename")
	}

	err = svc.DeleteFile("../etc", "file.png")
	if err == nil {
		t.Fatal("expected error for traversal batch")
	}

	err = svc.DeleteFile("batch", "../file.png")
	if err == nil {
		t.Fatal("expected error for traversal filename")
	}

	err = svc.DeleteFile("batch", "nonexistent.png")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestDeleteFile_Directory(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "batch", "subdir"), 0755)

	svc := NewScreenshotAppService(dir)
	err := svc.DeleteFile("batch", "subdir")
	if err == nil {
		t.Fatal("expected error when trying to delete a directory")
	}
}

// ===== CaptureBatch =====

func TestCaptureBatch_NoProvider(t *testing.T) {
	svc := NewScreenshotAppService("./screenshots")
	_, err := svc.CaptureBatch(context.Background(), nil, BatchScreenshotRequest{})
	if err == nil {
		t.Fatal("expected error when no provider")
	}
}

func TestCaptureBatch_Success(t *testing.T) {
	svc := NewScreenshotAppServiceWithProvider("./screenshots", &mockScreenshotProvider{})
	req := BatchScreenshotRequest{
		QueryID: "test-batch",
		Engines: []struct {
			Engine string
			Query  string
		}{{Engine: "fofa", Query: "test"}},
		Targets: []struct {
			URL      string
			IP       string
			Port     string
			Protocol string
		}{{URL: "http://example.com"}},
	}
	resp, err := svc.CaptureBatch(context.Background(), nil, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.QueryID != "test-batch" {
		t.Errorf("QueryID = %q, want %q", resp.QueryID, "test-batch")
	}
	if len(resp.SearchEngines) != 1 {
		t.Errorf("expected 1 search engine result, got %d", len(resp.SearchEngines))
	}
	if len(resp.Targets) != 1 {
		t.Errorf("expected 1 target result, got %d", len(resp.Targets))
	}
}
