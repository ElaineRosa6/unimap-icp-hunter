package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/unimap/project/internal/backup"
)

func TestBackupRequestCancellationPreservesRecoveryPoint(t *testing.T) {
	srv := setupBackupServer(t, t.TempDir())
	srv.config.Backup.MaxBackups = 1
	old, err := backup.Backup(backup.BackupConfig{Sources: srv.config.Backup.Sources, OutputDir: srv.config.Backup.OutputDir, Prefix: srv.config.Backup.Prefix})
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(old.Path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/create", nil).WithContext(ctx)
	req.Header.Set("Origin", "http://localhost:8448")
	rec := httptest.NewRecorder()
	srv.handleCreateBackup(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("cancelled backup HTTP=%d, want 500", rec.Code)
	}
	entries, err := os.ReadDir(srv.config.Backup.OutputDir)
	if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(old.Path) {
		t.Errorf("cancelled backup changed recovery points: %v %v", entries, err)
	}
	current, err := os.ReadFile(old.Path)
	if err != nil || string(current) != string(original) {
		t.Errorf("old recovery point changed: %v", err)
	}
}
