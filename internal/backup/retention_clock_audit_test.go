package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuditRetentionClockSkew(t *testing.T) {
	for _, future := range []bool{false, true} {
		name := "past-control"
		if future {
			name = "future-old-archive"
		}
		t.Run(name, func(t *testing.T) {
			source, output := t.TempDir(), t.TempDir()
			payload := filepath.Join(source, "payload")
			if err := os.WriteFile(payload, []byte("old"), 0600); err != nil {
				t.Fatal(err)
			}
			cfg := BackupConfig{Sources: []string{source}, OutputDir: output, Prefix: "audit", MaxBackups: 1}
			old, err := Backup(cfg)
			if err != nil {
				t.Fatal(err)
			}
			stamp := time.Now().Add(-24 * time.Hour)
			if future {
				stamp = time.Now().Add(24 * time.Hour)
			}
			if err = os.Chtimes(old.Path, stamp, stamp); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(payload, []byte("new"), 0600); err != nil {
				t.Fatal(err)
			}
			current, err := Backup(cfg)
			if err != nil {
				t.Fatal(err)
			}
			file, err := os.Open(current.Path)
			if err != nil {
				t.Fatalf("Backup returned success but published path is missing: %v", err)
			}
			defer file.Close()
			gz, err := gzip.NewReader(file)
			if err != nil {
				t.Fatal(err)
			}
			defer gz.Close()
			tr := tar.NewReader(gz)
			if _, err = tr.Next(); err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(tr)
			if err != nil || string(got) != "new" {
				t.Fatalf("restore got=%q err=%v", got, err)
			}
			if _, err := os.Stat(old.Path); !os.IsNotExist(err) {
				t.Fatalf("old recovery point retained: %v", err)
			}
		})
	}
}

func TestBackupRetentionPublicationSlots(t *testing.T) {
	for _, limit := range []int{0, 1, 2, 4} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			source, output := t.TempDir(), t.TempDir()
			if err := os.WriteFile(filepath.Join(source, "payload"), []byte("payload"), 0600); err != nil {
				t.Fatal(err)
			}
			cfg := BackupConfig{Sources: []string{source}, OutputDir: output, Prefix: "audit"}
			var old []string
			for i := 0; i < 3; i++ {
				b, err := Backup(cfg)
				if err != nil {
					t.Fatal(err)
				}
				stamp := time.Now().Add(time.Duration(i+1) * time.Hour)
				if err := os.Chtimes(b.Path, stamp, stamp); err != nil {
					t.Fatal(err)
				}
				old = append(old, b.Path)
			}
			cfg.MaxBackups = limit
			current, err := Backup(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(current.Path); err != nil {
				t.Fatal(err)
			}
			listed, err := ListBackups(output, "audit")
			want := limit
			if limit == 0 {
				want = 4
			}
			if err != nil || len(listed) != want {
				t.Fatalf("count=%d want=%d err=%v", len(listed), want, err)
			}
			if limit == 2 {
				if _, err := os.Stat(old[2]); err != nil {
					t.Fatalf("newest remaining recovery point missing: %v", err)
				}
			}
		})
	}
}

func TestBackupRetentionEqualTimes(t *testing.T) {
	output := t.TempDir()
	stamp := time.Now().Add(-time.Hour)
	var names []string
	for _, day := range []string{"01", "02", "03"} {
		name := filepath.Join(output, "audit_backup_202501"+day+"_120000.tar.gz")
		if err := os.WriteFile(name, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(name, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	// Directory order places this last; equal mtimes must not evict it.
	cleanupOldBackups(output, "audit", 1, names[2])
	listed, err := ListBackups(output, "audit")
	if err != nil || len(listed) != 1 || listed[0].Path != names[2] {
		t.Fatalf("publication not protected: %v %v", listed, err)
	}
}
