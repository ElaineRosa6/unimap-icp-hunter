package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupFailureHTTPRejectsMissingConfiguredSource(t *testing.T) {
	dir := t.TempDir()
	s := setupBackupServer(t, dir)
	s.config.Backup.MaxBackups = 1
	create := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/create", nil)
		req.Host = "localhost:8448"
		req.Header.Set("Origin", "http://localhost:8448")
		w := httptest.NewRecorder()
		s.handleCreateBackup(w, req)
		return w
	}
	if w := create(); w.Code != 201 {
		t.Fatalf("initial backup: %d %s", w.Code, w.Body.String())
	}
	before, err := os.ReadDir(s.config.Backup.OutputDir)
	if err != nil {
		t.Fatal(err)
	}
	s.config.Backup.Sources = append(s.config.Backup.Sources, filepath.Join(dir, "missing-source"))
	if w := create(); w.Code != 500 {
		t.Errorf("missing configured source ignored: %d %s", w.Code, w.Body.String())
	}
	after, err := os.ReadDir(s.config.Backup.OutputDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 || len(after) != 1 || before[0].Name() != after[0].Name() {
		t.Error("failed HTTP backup replaced the last complete archive")
	}
}

func TestBackupExplicitFileSource(t *testing.T) {
	dir := t.TempDir()
	s := setupBackupServer(t, dir)
	s.config.Backup.Sources = []string{filepath.Join(dir, "data", "tasks.json")}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/create", nil)
	req.Host = "localhost:8448"
	req.Header.Set("Origin", "http://localhost:8448")
	w := httptest.NewRecorder()
	s.handleCreateBackup(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("explicit file source: %d %s", w.Code, w.Body.String())
	}
}
