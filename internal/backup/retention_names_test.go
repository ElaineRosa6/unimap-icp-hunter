package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupRetentionPrefixIsolation(t *testing.T) {
	source := t.TempDir()
	output := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "payload"), []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	other, err := Backup(BackupConfig{Sources: []string{source}, OutputDir: output, Prefix: "unimap-other"})
	if err != nil {
		t.Fatal(err)
	}
	protected := map[string][]byte{}
	bytes, err := os.ReadFile(other.Path)
	if err != nil {
		t.Fatal(err)
	}
	protected[other.Path] = bytes
	for _, name := range []string{"unimap-personal.tar.gz", "unimap_backup_notes.tar.gz", "unimap_backup_20250230_120000.tar.gz", "unimap_backup_20250101_120000.000000000_not-a-suffix.tar.gz"} {
		path := filepath.Join(output, name)
		data := []byte("preserve unrelated archive")
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		protected[path] = data
	}
	// Ensure these files are older than the new archive, independent of test clock granularity.
	oldTime := time.Now().Add(-time.Hour)
	for path := range protected {
		if err = os.Chtimes(path, oldTime, oldTime); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := ListBackups(output, "unimap")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Errorf("unrelated entries listed under unimap: count=%d", len(listed))
	}
	current, err := Backup(BackupConfig{Sources: []string{source}, OutputDir: output, Prefix: "unimap", MaxBackups: 1})
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range protected {
		got, readErr := os.ReadFile(path)
		if readErr != nil || string(got) != string(want) {
			t.Errorf("unrelated archive removed or changed: %s: %v", filepath.Base(path), readErr)
		}
	}
	listed, err = ListBackups(output, "unimap")
	if err != nil || len(listed) != 1 || listed[0].Path != current.Path {
		t.Errorf("own archive not listed: %v %v", listed, err)
	}
}

func TestBackupRetentionLegacyNames(t *testing.T) {
	source := t.TempDir()
	output := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "payload"), []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(output, "unimap_backup_20250101_120000.tar.gz")
	if err := os.WriteFile(legacy, []byte("legacy archive fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(legacy, old, old); err != nil {
		t.Fatal(err)
	}
	listed, err := ListBackups(output, "unimap")
	if err != nil || len(listed) != 1 {
		t.Fatalf("legacy archive missing: %v %v", listed, err)
	}
	if _, err = Backup(BackupConfig{Sources: []string{source}, OutputDir: output, MaxBackups: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy archive not rotated: %v", err)
	}
}

func TestManagedBackupNameFormats(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"unimap_backup_20250101_120000.tar.gz", true},
		{"unimap_backup_20240229_235959.123456789_0.tar.gz", true},
		{"unimap_backup_20250101_120000.000000000_4294967295.tar.gz", true},
		{"unimap-extra_backup_20250101_120000.tar.gz", false},
		{"unimap_backup_20250101_120000_extra.tar.gz", false},
		{"unimap_backup_20250101_240000.tar.gz", false},
		{"unimap_backup_20250229_120000.tar.gz", false},
		{"unimap_backup_20250101_120000.000000000_4294967296.tar.gz", false},
		{"unimap_backup_20250101_120000.000000000_01.tar.gz", false},
		{"unimap_backup_20250101_120000.000000000_+1.tar.gz", false},
		{"unimap_backup_20250101_120000.00000000a_1.tar.gz", false},
		{"unimap_backup_20250101_120000.000000000_.tar.gz", false},
		{"unimap_backup_.tar.gz", false},
		{"unimap_backup_20250101_120000.tar.gz.tmp", false},
	} {
		if got := isManagedBackupName(tc.name, "unimap"); got != tc.want {
			t.Errorf("name=%q got=%v want=%v", tc.name, got, tc.want)
		}
	}
	if !isManagedBackupName("literal[+]_backup_20250101_120000.tar.gz", "literal[+]") {
		t.Fatal("prefix was not treated literally")
	}
}

func TestBackupRetentionLeavesLinks(t *testing.T) {
	output := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(output, "unimap_backup_20250101_120000.tar.gz")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink support required: %v", err)
	}
	list, err := ListBackups(output, "unimap")
	if err != nil || len(list) != 0 {
		t.Fatalf("link listed as managed archive: %v %v", list, err)
	}
	cleanupOldBackups(output, "unimap", 0)
	if info, statErr := os.Lstat(link); statErr != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link removed: %v", statErr)
	}
	if data, readErr := os.ReadFile(target); readErr != nil || string(data) != "keep" {
		t.Fatalf("link target changed: %v", readErr)
	}
}
