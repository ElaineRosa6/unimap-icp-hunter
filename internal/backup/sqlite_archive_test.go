//go:build cgo

package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type archiveSQLiteOwner struct {
	db   *sql.DB
	info os.FileInfo
	path string
}

func newArchiveSQLiteOwner(t *testing.T, dir string) *archiveSQLiteOwner {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "live.sqlite")
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=50")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	for _, q := range []string{"PRAGMA wal_autocheckpoint=0", "CREATE TABLE marker(value TEXT)", "PRAGMA wal_checkpoint(TRUNCATE)", "INSERT INTO marker(rowid,value) VALUES(42,'committed')"} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return &archiveSQLiteOwner{db: db, info: info, path: path}
}
func archiveSnapshotter(owners ...*archiveSQLiteOwner) SQLiteSnapshotter {
	return func(ctx context.Context, info os.FileInfo, destination string) error {
		for _, owner := range owners {
			if os.SameFile(info, owner.info) {
				return SnapshotSQLite(ctx, owner.db, destination)
			}
		}
		return fmt.Errorf("no owner for SQLite source")
	}
}
func assertArchivedSQLiteRow(t *testing.T, bytes string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "restored.sqlite")
	if err := os.WriteFile(path, []byte(bytes), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", path+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var got int
	if err = db.QueryRow("SELECT count(*) FROM marker WHERE rowid=42 AND value='committed'").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Fatalf("restored_rows=%d want 1", got)
	}
}
func TestSQLiteArchiveSnapshotRestoresWAL(t *testing.T) {
	for _, mode := range []string{"file", "nested-directory"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			owner := newArchiveSQLiteOwner(t, filepath.Join(dir, "nested"))
			source := dir
			if mode == "file" {
				source = owner.path
			}
			output := t.TempDir()
			result, err := BackupContext(context.Background(), BackupConfig{Sources: []string{source}, OutputDir: output, SQLiteSnapshotter: archiveSnapshotter(owner)})
			if err != nil {
				t.Fatal(err)
			}
			entries := readBoundaryArchive(t, result.Path)
			name := "nested/live.sqlite"
			if mode == "file" {
				name = "live.sqlite"
			}
			if len(entries) != 1 {
				t.Fatalf("snapshot archive includes companions or staging paths: %v", entries)
			}
			assertArchivedSQLiteRow(t, entries[name])
			dirEntries, err := os.ReadDir(output)
			if err != nil || len(dirEntries) != 1 {
				t.Fatalf("staging leaked: %v %v", dirEntries, err)
			}
		})
	}
}
func TestSQLiteArchiveMultipleDatabasesAndUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	one := newArchiveSQLiteOwner(t, filepath.Join(dir, "one"))
	two := newArchiveSQLiteOwner(t, filepath.Join(dir, "two"))
	for _, name := range []string{"notes-wal", "notes-shm", "live.sqlite-wal.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := BackupContext(context.Background(), BackupConfig{Sources: []string{dir}, OutputDir: t.TempDir(), SQLiteSnapshotter: archiveSnapshotter(one, two)})
	if err != nil {
		t.Fatal(err)
	}
	entries := readBoundaryArchive(t, result.Path)
	if len(entries) != 5 {
		t.Fatalf("archive count=%d want 5", len(entries))
	}
	assertArchivedSQLiteRow(t, entries["one/live.sqlite"])
	assertArchivedSQLiteRow(t, entries["two/live.sqlite"])
	for _, name := range []string{"notes-wal", "notes-shm", "live.sqlite-wal.txt"} {
		if entries[name] != "keep" {
			t.Fatalf("unrelated file dropped: %s", name)
		}
	}
}
func TestSQLiteArchiveFailurePreservesRecoveryPoint(t *testing.T) {
	for _, mode := range []string{"unknown-owner", "callback-failure", "cancelled", "invalid-output", "invalid-file"} {
		t.Run(mode, func(t *testing.T) {
			owner := newArchiveSQLiteOwner(t, t.TempDir())
			output := t.TempDir()
			cfg := BackupConfig{Sources: []string{owner.path}, OutputDir: output, MaxBackups: 1, SQLiteSnapshotter: archiveSnapshotter(owner)}
			old, err := BackupContext(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(old.Path)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			sentinel := errors.New("snapshot failure")
			switch mode {
			case "unknown-owner":
				cfg.SQLiteSnapshotter = archiveSnapshotter()
			case "callback-failure":
				cfg.SQLiteSnapshotter = func(_ context.Context, _ os.FileInfo, path string) error {
					if writeErr := os.WriteFile(path, []byte("partial"), 0600); writeErr != nil {
						return writeErr
					}
					return sentinel
				}
			case "cancelled":
				cfg.SQLiteSnapshotter = func(c context.Context, info os.FileInfo, path string) error {
					cancel()
					return archiveSnapshotter(owner)(c, info, path)
				}
			case "invalid-file":
				cfg.SQLiteSnapshotter = func(_ context.Context, _ os.FileInfo, path string) error {
					return os.WriteFile(path, []byte("not a SQLite file despite enough bytes"), 0600)
				}
			case "invalid-output":
				cfg.SQLiteSnapshotter = func(_ context.Context, _ os.FileInfo, path string) error { return os.Mkdir(path, 0700) }
			}
			result, err := BackupContext(ctx, cfg)
			if result != nil || err == nil {
				t.Fatalf("bad snapshot published: %v %v", result, err)
			}
			if mode == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost cancellation: %v", err)
			}
			if mode == "callback-failure" && !errors.Is(err, sentinel) {
				t.Fatalf("lost callback error: %v", err)
			}
			after, readErr := os.ReadFile(old.Path)
			if readErr != nil || string(after) != string(before) {
				t.Fatalf("old recovery point changed: %v", readErr)
			}
			entries, readErr := os.ReadDir(output)
			if readErr != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(old.Path) {
				t.Fatalf("staging or partial archive leaked: %v %v", entries, readErr)
			}
		})
	}
}
func TestSQLiteArchiveUnselectedCompanionIsRetained(t *testing.T) {
	owner := newArchiveSQLiteOwner(t, t.TempDir())
	calls := 0
	result, err := BackupContext(context.Background(), BackupConfig{Sources: []string{owner.path + "-wal"}, OutputDir: t.TempDir(), SQLiteSnapshotter: func(context.Context, os.FileInfo, string) error {
		calls++
		return fmt.Errorf("unexpected SQLite source")
	}})
	if err != nil {
		t.Fatal(err)
	}
	entries := readBoundaryArchive(t, result.Path)
	if len(entries) != 1 || len(entries["live.sqlite-wal"]) == 0 || calls != 0 {
		t.Fatalf("unselected companion discarded; entries=%d calls=%d", len(entries), calls)
	}
}

func TestSQLiteArchiveConcurrentTransactions(t *testing.T) {
	owner := newArchiveSQLiteOwner(t, t.TempDir())
	owner.db.SetMaxOpenConns(2)
	if _, err := owner.db.Exec("CREATE TABLE balances (id INTEGER PRIMARY KEY, total INTEGER); INSERT INTO balances VALUES(1,100),(2,0)"); err != nil {
		t.Fatal(err)
	}
	writerCtx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		first := true
		for i := 0; ; i++ {
			if writerCtx.Err() != nil {
				done <- nil
				return
			}
			tx, err := owner.db.BeginTx(writerCtx, nil)
			if err != nil {
				if writerCtx.Err() != nil {
					done <- nil
				} else {
					done <- err
				}
				return
			}
			if _, err = tx.ExecContext(writerCtx, "UPDATE balances SET total=? WHERE id=1", i%101); err == nil {
				_, err = tx.ExecContext(writerCtx, "UPDATE balances SET total=? WHERE id=2", 100-i%101)
			}
			if err == nil {
				err = tx.Commit()
			} else {
				_ = tx.Rollback()
			}
			if err != nil {
				if writerCtx.Err() != nil {
					done <- nil
				} else {
					done <- err
				}
				return
			}
			if first {
				close(started)
				first = false
			}
		}
	}()
	defer func() {
		stop()
		if err := <-done; err != nil {
			t.Errorf("writer failed: %v", err)
		}
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not commit")
	}
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		result, err := BackupContext(ctx, BackupConfig{Sources: []string{owner.path}, OutputDir: t.TempDir(), SQLiteSnapshotter: archiveSnapshotter(owner)})
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		entries := readBoundaryArchive(t, result.Path)
		assertArchivedSQLiteRow(t, entries["live.sqlite"])
		path := filepath.Join(t.TempDir(), "restored.sqlite")
		if err = os.WriteFile(path, []byte(entries["live.sqlite"]), 0600); err != nil {
			t.Fatal(err)
		}
		restored, err := sql.Open("sqlite3", path)
		if err != nil {
			t.Fatal(err)
		}
		var count, sum int
		queryErr := restored.QueryRow("SELECT count(*),sum(total) FROM balances").Scan(&count, &sum)
		closeErr := restored.Close()
		if queryErr != nil || closeErr != nil || count != 2 || sum != 100 {
			t.Fatalf("torn transaction: count=%d sum=%d query=%v close=%v", count, sum, queryErr, closeErr)
		}
	}
}

func TestSQLiteArchiveRejectsEscapingLink(t *testing.T) {
	owner := newArchiveSQLiteOwner(t, t.TempDir())
	source := t.TempDir()
	output := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "keep.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	old, err := BackupContext(context.Background(), BackupConfig{Sources: []string{source}, OutputDir: output})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(old.Path)
	if err != nil {
		t.Fatal(err)
	}
	if linkErr := os.Symlink(owner.path, filepath.Join(source, "outside.sqlite")); linkErr != nil {
		t.Skipf("symlink support required: %v", linkErr)
	}
	calls := 0
	result, err := BackupContext(context.Background(), BackupConfig{Sources: []string{source}, OutputDir: output, MaxBackups: 1, SQLiteSnapshotter: func(context.Context, os.FileInfo, string) error {
		calls++
		return fmt.Errorf("unexpected outside source")
	}})
	if err == nil || result != nil || calls != 0 {
		t.Fatalf("outside link reached callback: result=%v err=%v calls=%d", result, err, calls)
	}
	after, err := os.ReadFile(old.Path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("old recovery point changed: %v", err)
	}
	entries, err := os.ReadDir(output)
	if err != nil || len(entries) != 1 {
		t.Fatalf("staging or partial archive leaked: %v %v", entries, err)
	}
}

func TestSQLiteArchiveDatabaseNamedLikeCompanionIsRetained(t *testing.T) {
	dir := t.TempDir()
	var owners []*archiveSQLiteOwner
	for _, name := range []string{"main.sqlite", "main.sqlite-wal"} {
		path := filepath.Join(dir, name)
		db, err := sql.Open("sqlite3", path+"?_journal_mode=DELETE")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		if _, err = db.Exec("CREATE TABLE marker(value TEXT); INSERT INTO marker(rowid,value) VALUES(42,'committed')"); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		owners = append(owners, &archiveSQLiteOwner{db: db, info: info, path: path})
	}
	result, err := BackupContext(context.Background(), BackupConfig{Sources: []string{dir}, OutputDir: t.TempDir(), SQLiteSnapshotter: archiveSnapshotter(owners...)})
	if err != nil {
		t.Fatal(err)
	}
	entries := readBoundaryArchive(t, result.Path)
	if len(entries) != 2 {
		t.Fatalf("a selected SQLite database was mistaken for a companion: entries=%d want 2", len(entries))
	}
	assertArchivedSQLiteRow(t, entries["main.sqlite"])
	assertArchivedSQLiteRow(t, entries["main.sqlite-wal"])
}
