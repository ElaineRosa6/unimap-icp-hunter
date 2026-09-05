package backup

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupFailurePreservesLastCompleteArchive(t *testing.T) {
	for _, failure := range []string{"missing-source", "unreadable-entry"} {
		t.Run(failure, func(t *testing.T) {
			src, out := t.TempDir(), t.TempDir()
			if err := os.WriteFile(filepath.Join(src, "good.txt"), []byte("complete data"), 0600); err != nil {
				t.Fatal(err)
			}
			cfg := BackupConfig{Sources: []string{src}, OutputDir: out, Prefix: "fixture", MaxBackups: 1}
			good, err := Backup(cfg)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(good.Path)
			if err != nil {
				t.Fatal(err)
			}
			missing := filepath.Join(t.TempDir(), "absent")
			if failure == "missing-source" {
				cfg.Sources = append(cfg.Sources, missing)
			} else {
				if linkErr := os.Symlink(missing, filepath.Join(src, "unreadable")); linkErr != nil {
					t.Fatal(linkErr)
				}
			}
			result, err := Backup(cfg)
			if err == nil || result != nil {
				t.Errorf("partial backup reported/published: result=%v error=%v", result, err)
			}
			after, err := os.ReadFile(good.Path)
			if err != nil || !bytes.Equal(before, after) {
				t.Errorf("last complete backup lost or changed: %v", err)
			}
			entries, err := os.ReadDir(out)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != filepath.Base(good.Path) {
				t.Errorf("failed backup left output or pruned good archive: %v", entries)
			}
		})
	}
}
