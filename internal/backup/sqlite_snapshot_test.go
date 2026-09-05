//go:build cgo

package backup

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func snapshotFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "live.sqlite")+"?_journal_mode=WAL&_busy_timeout=50")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	for _, q := range []string{"PRAGMA wal_autocheckpoint=0", "CREATE TABLE state (value TEXT)", "PRAGMA wal_checkpoint(TRUNCATE)", "INSERT INTO state(rowid,value) VALUES(42,'committed marker')"} {
		if _, execErr := db.Exec(q); execErr != nil {
			t.Fatal(execErr)
		}
	}
	return db
}

func TestSQLiteSnapshotRetainsCommittedWALAndRowID(t *testing.T) {
	db := snapshotFixture(t)
	target := filepath.Join(t.TempDir(), "snapshot #%.sqlite")
	if err := SnapshotSQLite(context.Background(), db, target); err != nil {
		t.Fatal(err)
	}
	archive, archiveErr := Backup(BackupConfig{Sources: []string{target}, OutputDir: t.TempDir()})
	if archiveErr != nil {
		t.Fatal(archiveErr)
	}
	entries := readBoundaryArchive(t, archive.Path)
	restoredPath := filepath.Join(t.TempDir(), "restored.sqlite")
	if writeErr := os.WriteFile(restoredPath, []byte(entries[filepath.Base(target)]), 0600); writeErr != nil {
		t.Fatal(writeErr)
	}
	uriPath := filepath.ToSlash(restoredPath)
	if filepath.VolumeName(restoredPath) != "" && !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	uri := url.URL{Scheme: "file", Path: uriPath}
	restored, err := sql.Open("sqlite3", uri.String())
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var id int
	var value string
	if queryErr := restored.QueryRow("SELECT rowid,value FROM state").Scan(&id, &value); queryErr != nil {
		t.Fatal(queryErr)
	}
	if id != 42 || value != "committed marker" {
		t.Fatalf("snapshot changed rows: %d %q", id, value)
	}
	info, err := os.Stat(target)
	if err != nil || info.Size() == 0 {
		t.Fatalf("snapshot file: %v", err)
	}
	var live string
	if queryErr := db.QueryRow("SELECT value FROM state WHERE rowid=42").Scan(&live); queryErr != nil || live != value {
		t.Fatalf("source changed: %q %v", live, queryErr)
	}
}

func TestSQLiteSnapshotFailureCleanup(t *testing.T) {
	for _, kind := range []string{"cancelled", "pool-timeout", "closed-source", "existing-target"} {
		t.Run(kind, func(t *testing.T) {
			db := snapshotFixture(t)
			dir := t.TempDir()
			target := filepath.Join(dir, "snapshot.sqlite")
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			switch kind {
			case "cancelled":
				cancel()
			case "pool-timeout":
				held, err := db.Conn(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				defer held.Close()
			case "closed-source":
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			case "existing-target":
				if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err := SnapshotSQLite(ctx, db, target)
			if err == nil {
				t.Fatal("expected snapshot failure")
			}
			if kind == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost cancellation: %v", err)
			}
			if kind == "pool-timeout" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("lost deadline: %v", err)
			}
			if kind == "existing-target" {
				data, readErr := os.ReadFile(target)
				if readErr != nil || string(data) != "preserve" {
					t.Fatalf("existing target changed: %v", readErr)
				}
			} else {
				entries, readErr := os.ReadDir(dir)
				if readErr != nil || len(entries) != 0 {
					t.Fatalf("snapshot residue: %v %v", entries, readErr)
				}
			}
		})
	}
}

func TestSQLiteSnapshotBusySourceCancellation(t *testing.T) {
	db := snapshotFixture(t)
	if _, err := db.Exec("PRAGMA journal_mode=DELETE"); err != nil {
		t.Fatal(err)
	}
	var seq int
	var schema, path string
	if err := db.QueryRow("PRAGMA database_list").Scan(&seq, &schema, &path); err != nil {
		t.Fatal(err)
	}
	writer, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	writer.SetMaxOpenConns(1)
	if _, beginErr := writer.Exec("BEGIN EXCLUSIVE"); beginErr != nil {
		t.Fatal(beginErr)
	}
	defer func() { _, _ = writer.Exec("ROLLBACK") }()
	dir := t.TempDir()
	target := filepath.Join(dir, "snapshot.sqlite")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if snapshotErr := SnapshotSQLite(ctx, db, target); !errors.Is(snapshotErr, context.DeadlineExceeded) {
		t.Fatalf("busy source did not preserve deadline: %v", snapshotErr)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatalf("busy snapshot residue: %v %v", files, err)
	}
}
