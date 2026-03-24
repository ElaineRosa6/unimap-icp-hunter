package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/config"
)

func buildTestServerWithScreenshotBase(baseDir string) *Server {
	cfg := &config.Config{}
	cfg.Screenshot.BaseDir = baseDir
	return &Server{config: cfg}
}

func TestNormalizeScreenshotPathToken(t *testing.T) {
	tests := []struct {
		in   string
		ok   bool
		want string
	}{
		{in: "batch-001", ok: true, want: "batch-001"},
		{in: "", ok: false},
		{in: "..", ok: false},
		{in: "a/b", ok: false},
		{in: "a\\b", ok: false},
	}

	for _, tt := range tests {
		got, ok := normalizeScreenshotPathToken(tt.in)
		if ok != tt.ok {
			t.Fatalf("input %q expected ok=%v got %v", tt.in, tt.ok, ok)
		}
		if ok && got != tt.want {
			t.Fatalf("input %q expected %q got %q", tt.in, tt.want, got)
		}
	}
}

func TestHandleScreenshotBatchesAndFiles(t *testing.T) {
	baseDir := t.TempDir()
	batchDir := filepath.Join(baseDir, "batch-a")
	if err := os.MkdirAll(batchDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(batchDir, "x.png"), []byte("x"), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	s := buildTestServerWithScreenshotBase(baseDir)

	req := httptest.NewRequest(http.MethodGet, "/api/screenshot/batches", nil)
	w := httptest.NewRecorder()
	s.handleScreenshotBatches(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var batchResp struct {
		Success bool `json:"success"`
		Count   int  `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &batchResp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if !batchResp.Success || batchResp.Count != 1 {
		t.Fatalf("unexpected batch response: %+v", batchResp)
	}

	reqFiles := httptest.NewRequest(http.MethodGet, "/api/screenshot/batches/files?batch=batch-a", nil)
	wFiles := httptest.NewRecorder()
	s.handleScreenshotBatchFiles(wFiles, reqFiles)
	if wFiles.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wFiles.Code)
	}

	var filesResp struct {
		Success bool `json:"success"`
		Count   int  `json:"count"`
	}
	if err := json.Unmarshal(wFiles.Body.Bytes(), &filesResp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if !filesResp.Success || filesResp.Count != 1 {
		t.Fatalf("unexpected files response: %+v", filesResp)
	}
}

func TestHandleScreenshotDeleteSafety(t *testing.T) {
	baseDir := t.TempDir()
	batchDir := filepath.Join(baseDir, "batch-z")
	if err := os.MkdirAll(batchDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(batchDir, "a.png"), []byte("x"), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	s := buildTestServerWithScreenshotBase(baseDir)

	badReq := httptest.NewRequest(http.MethodDelete, "/api/screenshot/file/delete?batch=../evil&file=a.png", nil)
	badW := httptest.NewRecorder()
	s.handleScreenshotFileDelete(badW, badReq)
	if badW.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for traversal, got %d", badW.Code)
	}

	okReq := httptest.NewRequest(http.MethodDelete, "/api/screenshot/file/delete?batch=batch-z&file=a.png", nil)
	okW := httptest.NewRecorder()
	s.handleScreenshotFileDelete(okW, okReq)
	if okW.Code != http.StatusOK {
		t.Fatalf("expected 200 for delete, got %d", okW.Code)
	}
	if _, err := os.Stat(filepath.Join(batchDir, "a.png")); !os.IsNotExist(err) {
		t.Fatalf("expected file deleted, stat err=%v", err)
	}
}
