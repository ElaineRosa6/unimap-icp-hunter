package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unimap-icp-hunter/project/internal/service"
	"github.com/unimap-icp-hunter/project/internal/tamper"
)

func TestHandleTamperHistoryDelete(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	url := "https://example.com"
	storage := tamper.NewHashStorage("./hash_store")
	if err := storage.SaveCheckRecord(url, &tamper.CheckRecord{
		URL:       url,
		CheckType: "normal",
		Timestamp: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("save record failed: %v", err)
	}

	recordsBase := filepath.Join("hash_store", "records")
	entries, err := os.ReadDir(recordsBase)
	if err != nil {
		t.Fatalf("read records base failed: %v", err)
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		t.Fatalf("expected exactly one url directory, got %d", len(entries))
	}
	recordsDir := filepath.Join(recordsBase, entries[0].Name())

	s := &Server{tamperApp: service.NewTamperAppService("./hash_store", nil)}

	missingReq := httptest.NewRequest(http.MethodDelete, "/api/tamper/history/delete", nil)
	missingW := httptest.NewRecorder()
	s.handleTamperHistoryDelete(missingW, missingReq)
	if missingW.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing url, got %d", missingW.Code)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/tamper/history/delete?url=https://example.com", nil)
	w := httptest.NewRecorder()
	s.handleTamperHistoryDelete(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Success bool   `json:"success"`
		URL     string `json:"url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if !resp.Success || resp.URL != url {
		t.Fatalf("unexpected response: %+v", resp)
	}

	if _, err := os.Stat(recordsDir); !os.IsNotExist(err) {
		t.Fatalf("expected records dir removed, stat err=%v", err)
	}
}

func TestHandleTamperBaselineDeleteMethodContract(t *testing.T) {
	s := &Server{tamperApp: service.NewTamperAppService("./hash_store", nil)}

	req := httptest.NewRequest(http.MethodPost, "/api/tamper/baseline/delete?url=https://example.com", nil)
	w := httptest.NewRecorder()
	s.handleTamperBaselineDelete(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for non-DELETE method, got %d", w.Code)
	}
}

func TestHandleTamperBaselineDeleteByQueryParam(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	targetURL := "https://example.com"
	detector := tamper.NewDetector(tamper.DetectorConfig{BaseDir: "./hash_store"})
	if err := detector.SaveBaseline(targetURL, &tamper.PageHashResult{URL: targetURL, FullHash: "baseline-hash"}); err != nil {
		t.Fatalf("save baseline failed: %v", err)
	}

	if urls, err := detector.ListBaselines(); err != nil {
		t.Fatalf("list baselines failed: %v", err)
	} else if len(urls) != 1 {
		t.Fatalf("expected 1 baseline before delete, got %d", len(urls))
	}

	s := &Server{tamperApp: service.NewTamperAppService("./hash_store", nil)}

	req := httptest.NewRequest(http.MethodDelete, "/api/tamper/baseline/delete?url=https://example.com", nil)
	w := httptest.NewRecorder()
	s.handleTamperBaselineDelete(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if urls, err := detector.ListBaselines(); err != nil {
		t.Fatalf("list baselines after delete failed: %v", err)
	} else if len(urls) != 0 {
		t.Fatalf("expected 0 baselines after delete, got %d", len(urls))
	}
}
