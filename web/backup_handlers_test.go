package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/config"
)

func setupBackupServer(t *testing.T, tmpDir string) *Server {
	t.Helper()
	hsDir := filepath.Join(tmpDir, "hash_store")
	dataDir := filepath.Join(tmpDir, "data")
	backupDir := filepath.Join(tmpDir, "backups")

	os.MkdirAll(hsDir, 0755)
	os.MkdirAll(dataDir, 0755)
	os.MkdirAll(backupDir, 0755)

	// 创建测试文件
	os.WriteFile(filepath.Join(hsDir, "baseline.json"), []byte(`{"url":"test"}`), 0644)
	os.WriteFile(filepath.Join(dataDir, "tasks.json"), []byte(`[]`), 0644)

	cfg := &config.Config{}
	cfg.Backup.OutputDir = backupDir
	cfg.Backup.Prefix = "unimap"
	cfg.Backup.MaxBackups = 5
	cfg.Backup.Sources = []string{hsDir, dataDir}

	return &Server{config: cfg}
}

func TestHandleCreateBackup_NoConfig(t *testing.T) {
	s := &Server{config: nil}

	req := httptest.NewRequest(http.MethodPost, "/api/backup/create", nil)
	rec := httptest.NewRecorder()

	s.handleCreateBackup(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleCreateBackup_WithSources(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "backup_create_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	s := setupBackupServer(t, tmpDir)

	req := httptest.NewRequest(http.MethodPost, "/api/backup/create", nil)
	rec := httptest.NewRecorder()

	s.handleCreateBackup(rec, req)

	if rec.Code != http.StatusCreated {
		body := rec.Body.String()
		t.Fatalf("expected 201, got %d, body: %s", rec.Code, body)
	}
}

func TestHandleListBackups_Empty(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "backup_list_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	s := setupBackupServer(t, tmpDir)

	req := httptest.NewRequest(http.MethodGet, "/api/backup/list", nil)
	rec := httptest.NewRecorder()

	s.handleListBackups(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleListBackups_AfterCreate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "backup_roundtrip_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	s := setupBackupServer(t, tmpDir)

	// 先创建一个备份
	createReq := httptest.NewRequest(http.MethodPost, "/api/backup/create", nil)
	createRec := httptest.NewRecorder()
	s.handleCreateBackup(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", createRec.Code)
	}

	// 然后列出备份
	listReq := httptest.NewRequest(http.MethodGet, "/api/backup/list", nil)
	listRec := httptest.NewRecorder()
	s.handleListBackups(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", listRec.Code)
	}
}

func TestFormatBackupSize(t *testing.T) {
	tests := []struct {
		size int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1048576, "1.00 MB"},
		{1073741824, "1.00 GB"},
	}

	for _, tt := range tests {
		got := formatBackupSize(tt.size)
		if got != tt.want {
			t.Errorf("formatBackupSize(%d) = %q, want %q", tt.size, got, tt.want)
		}
	}
}

func TestDirExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "direxists_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	if !dirExists(tmpDir) {
		t.Error("dirExists should return true for existing dir")
	}

	if dirExists(filepath.Join(tmpDir, "nonexistent")) {
		t.Error("dirExists should return false for non-existent path")
	}

	// 创建文件而非目录
	f, _ := os.Create(filepath.Join(tmpDir, "file.txt"))
	f.Close()

	if dirExists(filepath.Join(tmpDir, "file.txt")) {
		t.Error("dirExists should return false for file")
	}
}
