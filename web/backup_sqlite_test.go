//go:build cgo

package web

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unimap/project/internal/auth"
	"github.com/unimap/project/internal/backup"
	"github.com/unimap/project/internal/config"
	history "github.com/unimap/project/internal/history"
	icp "github.com/unimap/project/internal/icp/database"
	"github.com/unimap/project/internal/model"
	"github.com/unimap/project/internal/scheduler"
	"github.com/unimap/project/internal/screenshot/batchdb"
	"github.com/unimap/project/internal/service"
)

func prepareOwnerMarker(t *testing.T, db *sql.DB) {
	t.Helper()
	db.SetMaxOpenConns(1)
	for _, q := range []string{"PRAGMA wal_autocheckpoint=0", "CREATE TABLE owner_marker(value TEXT)", "PRAGMA wal_checkpoint(TRUNCATE)", "INSERT INTO owner_marker(rowid,value) VALUES(42,'committed')"} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}
func restoreOwnerArchive(t *testing.T, path string) map[string][]byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	entries := map[string][]byte{}
	for {
		h, nextErr := tr.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			t.Fatal(nextErr)
		}
		body, readErr := io.ReadAll(tr)
		if readErr != nil {
			t.Fatal(readErr)
		}
		entries[h.Name] = body
	}
	return entries
}
func assertOwnerRows(t *testing.T, body []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "restore.sqlite")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err = db.QueryRow("SELECT count(*) FROM owner_marker WHERE rowid=42 AND value='committed'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("restored_rows=%d want 1, error=%v", count, err)
	}
}

func TestProductionSQLiteBackupEntrypoints(t *testing.T) {
	dir := t.TempDir()
	srv := &Server{config: &config.Config{}}
	h, err := history.NewDatabase(filepath.Join(dir, "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	srv.historyDB = h
	u, err := auth.NewUserDB(filepath.Join(dir, "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	srv.userDB = u
	b, err := batchdb.NewDatabase(filepath.Join(dir, "batches.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	srv.batchDB = b
	c, err := icp.NewDatabase(filepath.Join(dir, "icp.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	srv.icpDB = c
	app, err := service.NewPersistentTamperAppService(filepath.Join(dir, "tamper"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	srv.tamperApp = app
	td, _, err := app.HistoryDatabase()
	if err != nil {
		t.Fatal(err)
	}
	for _, db := range []*sql.DB{h.DB(), u.DB(), b.DB(), c.DB(), td} {
		prepareOwnerMarker(t, db)
	}
	initBackupSnapshots(srv)
	names := []string{"history.db", "users.db", "batches.db", "icp.db", "tamper/check_records.db"}
	for _, entry := range []string{"web", "runner"} {
		for _, name := range append(append([]string{}, names...), "all") {
			t.Run(entry+"/"+name, func(t *testing.T) {
				source := dir
				if name != "all" {
					source = filepath.Join(dir, filepath.FromSlash(name))
				}
				output := t.TempDir()
				srv.config.Backup.Sources = []string{source}
				srv.config.Backup.OutputDir = output
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if entry == "web" {
					req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/create", nil).WithContext(ctx)
					req.Header.Set("Origin", "http://localhost:8448")
					rec := httptest.NewRecorder()
					srv.handleCreateBackup(rec, req)
					if rec.Code != 201 {
						t.Fatalf("HTTP=%d: %s", rec.Code, rec.Body.String())
					}
				} else {
					_, err := scheduler.NewBackupRunner(srv.snapshotSQLite).Execute(ctx, &model.TaskPayload{Extra: map[string]any{"sources": []string{source}, "output_dir": output}})
					if err != nil {
						t.Fatal(err)
					}
				}
				archives, err := backup.ListBackups(output, "")
				if err != nil || len(archives) != 1 {
					t.Fatalf("archives=%d error=%v", len(archives), err)
				}
				entries := restoreOwnerArchive(t, archives[0].Path)
				want := 1
				if name == "all" {
					want = 5
				}
				if len(entries) != want {
					t.Fatalf("archive members=%d want %d (unexpected sidecars)", len(entries), want)
				}
				if name == "all" {
					for _, key := range names {
						assertOwnerRows(t, entries[key])
					}
				} else {
					assertOwnerRows(t, entries[filepath.Base(name)])
				}
				files, err := os.ReadDir(output)
				if err != nil || len(files) != 1 {
					t.Fatalf("staging leaked: %v %v", files, err)
				}
			})
		}
	}
}

func TestProductionSQLiteUnboundSourcePreservesBackup(t *testing.T) {
	for _, entry := range []string{"web", "runner"} {
		t.Run(entry, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "unknown.db")
			db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			prepareOwnerMarker(t, db)
			output := t.TempDir()
			plain := filepath.Join(t.TempDir(), "plain")
			if err = os.WriteFile(plain, []byte("old recovery point"), 0600); err != nil {
				t.Fatal(err)
			}
			old, err := backup.Backup(backup.BackupConfig{Sources: []string{plain}, OutputDir: output})
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(old.Path)
			if err != nil {
				t.Fatal(err)
			}
			if entry == "web" {
				cfg := &config.Config{}
				cfg.Backup.Sources = []string{path}
				cfg.Backup.OutputDir = output
				cfg.Backup.MaxBackups = 1
				srv := &Server{config: cfg}
				req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/create", nil)
				req.Header.Set("Origin", "http://localhost:8448")
				rec := httptest.NewRecorder()
				srv.handleCreateBackup(rec, req)
				if rec.Code != 500 {
					t.Fatalf("unknown SQLite HTTP=%d want 500", rec.Code)
				}
			} else {
				_, err = scheduler.NewBackupRunner().Execute(context.Background(), &model.TaskPayload{Extra: map[string]any{"sources": []string{path}, "output_dir": output, "max_backups": 1}})
				if err == nil {
					t.Fatal("unknown SQLite was silently copied")
				}
			}
			after, err := os.ReadFile(old.Path)
			if err != nil || string(after) != string(before) {
				t.Fatalf("old recovery point changed: %v", err)
			}
			files, err := os.ReadDir(output)
			if err != nil || len(files) != 1 {
				t.Fatalf("staging leaked: %v %v", files, err)
			}
		})
	}
}

func TestProductionSQLiteClosedAndBusyOwner(t *testing.T) {
	for _, entry := range []string{"web", "runner"} {
		for _, mode := range []string{"closed", "busy"} {
			t.Run(entry+"/"+mode, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "history.db")
				owner, err := history.NewDatabase(path)
				if err != nil {
					t.Fatal(err)
				}
				defer owner.Close()
				prepareOwnerMarker(t, owner.DB())
				output := t.TempDir()
				cfg := &config.Config{}
				cfg.Backup.Sources = []string{path}
				cfg.Backup.OutputDir = output
				cfg.Backup.MaxBackups = 1
				srv := &Server{config: cfg, historyDB: owner}
				initBackupSnapshots(srv)
				old, err := backup.BackupContext(context.Background(), backup.BackupConfig{Sources: []string{path}, OutputDir: output, SQLiteSnapshotter: srv.snapshotSQLite})
				if err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(old.Path)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "closed" {
					if err = owner.Close(); err != nil {
						t.Fatal(err)
					}
				} else {
					held, holdErr := owner.DB().Conn(context.Background())
					if holdErr != nil {
						t.Fatal(holdErr)
					}
					defer held.Close()
				}
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()
				if entry == "web" {
					req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/create", nil).WithContext(ctx)
					req.Header.Set("Origin", "http://localhost:8448")
					rec := httptest.NewRecorder()
					srv.handleCreateBackup(rec, req)
					if rec.Code != 500 {
						t.Fatalf("owner %s HTTP=%d", mode, rec.Code)
					}
				} else {
					_, err = scheduler.NewBackupRunner(srv.snapshotSQLite).Execute(ctx, &model.TaskPayload{Extra: map[string]any{"sources": []string{path}, "output_dir": output, "max_backups": 1}})
					if err == nil {
						t.Fatalf("owner %s reported backup success", mode)
					}
					if mode == "busy" && !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("deadline identity lost: %v", err)
					}
				}
				after, err := os.ReadFile(old.Path)
				if err != nil || string(after) != string(before) {
					t.Fatalf("old recovery point changed: %v", err)
				}
				files, err := os.ReadDir(output)
				if err != nil || len(files) != 1 {
					t.Fatalf("staging leaked: %v %v", files, err)
				}
			})
		}
	}
}
